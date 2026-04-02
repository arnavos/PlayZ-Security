package commands

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/summrs-dev-team/summrs-premium/database"
	"github.com/summrs-dev-team/summrs-premium/utils"
)

func (cmds *Commands) Add(name string, function handler, config *Config) *Command {
	cmd := Command{Name: name, Run: function, Config: config}
	cmds.Commands = append(cmds.Commands, &cmd)
	return &cmd
}

func (cmds *Commands) addCooldown(userID, command string, cooldown int) {
	cmds.Cooldown.Mutex.Lock()
	cmds.Cooldown.Cooldowns[userID] = append(cmds.Cooldown.Cooldowns[userID], command)
	cmds.Cooldown.Mutex.Unlock()

	time.AfterFunc(time.Duration(cooldown)*time.Second, func() {
		cmds.Cooldown.Mutex.Lock()
		cmds.Cooldown.Cooldowns[userID] = utils.RemoveFromSlice(cmds.Cooldown.Cooldowns[userID], command)
		cmds.Cooldown.Mutex.Unlock()
	})
}

func (cmds *Commands) hasCooldown(userID, command string) bool {
	cmds.Cooldown.Mutex.RLock()
	defer cmds.Cooldown.Mutex.RUnlock()
	return utils.FindInSlice(cmds.Cooldown.Cooldowns[userID], command)
}

func (cmds *Commands) Match(s *discordgo.Session, raw *discordgo.Message, context *Context) (*Command, []string) {
	fields := strings.Fields(context.Content)
	if len(fields) == 0 {
		return nil, nil
	}

	collection, err := database.Database.FindData(raw.GuildID)
	if err != nil {
		_, _ = s.ChannelMessageSend(raw.ChannelID, fmt.Sprintf("```Failed to get the database collection: %s```", err.Error()))
		return nil, nil
	}

	prefix, _ := collection["prefix"].(string)
	if prefix == "" {
		prefix = ">"
	}
	context.Prefix = prefix

	if !strings.HasPrefix(fields[0], context.Prefix) {
		return nil, nil
	}

	fields[0] = strings.ToLower(strings.TrimPrefix(fields[0], context.Prefix))
	for _, command := range cmds.Commands {
		if fields[0] != strings.ToLower(command.Name) && !matchAlias(command.Config.Alias, fields[0]) {
			continue
		}

		failure := ""
		switch {
		case cmds.hasCooldown(raw.Author.ID, command.Name):
			return nil, nil
		case !utils.HasPerms(s, raw, raw.GuildID, raw.Author.ID, command.Config.Perms):
			sendError(s, raw.ChannelID, "You do not have the required permissions to use this command.", raw.Author.Username)
			return nil, nil
		case command.Config.RequiresArgs && len(fields) < 2:
			failure = "You need args to use this command."
		case command.Config.RequiresMention && len(raw.Mentions) == 0:
			failure = "You have to mention someone to use this command."
		case command.Config.RequiresRoleMention && len(raw.MentionRoles) == 0:
			failure = "You have to mention a role to use this command."
		case command.Config.WhitelistedOnly &&
			!database.Database.IsWhitelisted(raw.GuildID, "users", raw.Author.ID, raw.Member) &&
			utils.GetGuildOwner(s, raw.GuildID) != raw.Author.ID:
			failure = "You have to be whitelisted to use this command."
		}

		if failure != "" {
			sendError(s, raw.ChannelID, failure, raw.Author.Username)
			return nil, nil
		}

		return command, fields
	}

	return nil, nil
}

func matchAlias(alias []string, target string) bool {
	for _, item := range alias {
		if strings.EqualFold(item, target) {
			return true
		}
	}
	return false
}

func (cmds *Commands) MessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot || m.GuildID == "" {
		return
	}

	ctx := &Context{Content: strings.TrimSpace(m.Content)}
	cmd, fields := cmds.Match(s, m.Message, ctx)
	if cmd == nil {
		return
	}

	ctx.Fields = fields[1:]
	cmd.Run(s, m.Message, ctx)
	cmds.addCooldown(m.Author.ID, cmd.Name, cmd.Config.Cooldown)
}

type (
	CommandCooldown struct {
		Mutex     *sync.RWMutex
		Cooldowns map[string][]string
	}

	Command struct {
		Name   string
		Run    handler
		Config *Config
	}

	Commands struct {
		Commands []*Command
		Cooldown *CommandCooldown
	}

	Config struct {
		Alias               []string
		Cooldown            int
		OwnerOnly           bool
		Perms               int
		RequiresArgs        bool
		RequiresMention     bool
		RequiresRoleMention bool
		WhitelistedOnly     bool
	}

	Context struct {
		Content string
		Prefix  string
		Fields  []string
	}

	handler func(*discordgo.Session, *discordgo.Message, *Context)
)
