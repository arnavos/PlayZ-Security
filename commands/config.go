package commands

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/summrs-dev-team/summrs-premium/database"
	"github.com/summrs-dev-team/summrs-premium/utils"
)

func (cmd *Commands) AntiInvite(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	state := strings.ToLower(ctx.Fields[0])
	if state != "on" && state != "off" {
		sendError(s, m.ChannelID, "Use `on` or `off`.", m.Author.Username)
		return
	}

	if _, err := database.Database.SetData("$set", m.GuildID, "anti-invite", state); err != nil {
		sendError(s, m.ChannelID, err.Error(), m.Author.Username)
		return
	}

	sendSuccess(s, m.ChannelID, fmt.Sprintf("Anti-Invite set to `%s`.", state), m.Author.Username)
}

func (cmd *Commands) AntiNuke(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	if len(ctx.Fields) == 0 {
		cmd.AntiNukeStatus(s, m, ctx)
		return
	}

	state := strings.ToLower(ctx.Fields[0])
	if state != "on" && state != "off" {
		sendError(s, m.ChannelID, "Use `on` or `off`, or run `antinuke` for status.", m.Author.Username)
		return
	}

	if !canManageAntiNuke(s, m) {
		sendError(s, m.ChannelID, "Only guild owner OR admin with role above bot can change anti-nuke toggle.", m.Author.Username)
		return
	}

	enabled := state == "on"
	if _, err := database.Database.SetToggle("$set", m.GuildID, "antinuke-enabled", enabled); err != nil {
		sendError(s, m.ChannelID, err.Error(), m.Author.Username)
		return
	}

	sendSuccess(s, m.ChannelID, fmt.Sprintf("Anti-nuke is now `%s`.", state), m.Author.Username)
}

func (cmd *Commands) AntiNukeStatus(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	data, err := database.Database.FindData(m.GuildID)
	if err != nil {
		sendError(s, m.ChannelID, err.Error(), m.Author.Username)
		return
	}

	threshold, _ := data["offense-threshold"].(string)
	if threshold == "" {
		threshold = "1"
	}

	mType, _ := data["moderation-type"].(string)
	if mType == "" {
		mType = "ban"
	}

	logChannel, _ := data["log-channel"].(string)
	if logChannel == "" || logChannel == "nil" {
		logChannel = "Not set"
	} else {
		logChannel = fmt.Sprintf("<#%s>", logChannel)
	}

	whitelistCount := 0
	if users, ok := data["users"].([]string); ok {
		whitelistCount = len(users)
	}

	moduleLines := []string{
		fmt.Sprintf("Anti Ban: %s", antiStatusEmoji(settingBool(data, "anti-ban", true))),
		fmt.Sprintf("Anti Kick: %s", antiStatusEmoji(settingBool(data, "anti-kick", true))),
		fmt.Sprintf("Anti Prune: %s", antiStatusEmoji(settingBool(data, "anti-prune", true))),
		fmt.Sprintf("Anti Bot: %s", antiStatusEmoji(settingBool(data, "anti-bot", true))),
		fmt.Sprintf("Anti Role Create: %s", antiStatusEmoji(settingBool(data, "anti-role-create", true))),
		fmt.Sprintf("Anti Role Delete: %s", antiStatusEmoji(settingBool(data, "anti-role-delete", true))),
		fmt.Sprintf("Anti Role Update: %s", antiStatusEmoji(settingBool(data, "anti-role-update", true))),
		fmt.Sprintf("Anti Channel Create: %s", antiStatusEmoji(settingBool(data, "anti-channel-create", true))),
		fmt.Sprintf("Anti Channel Delete: %s", antiStatusEmoji(settingBool(data, "anti-channel-delete", true))),
		fmt.Sprintf("Anti Channel Update: %s", antiStatusEmoji(settingBool(data, "anti-channel-update", true))),
		fmt.Sprintf("Anti Everyone Mention: %s", antiStatusEmoji(settingBool(data, "anti-everyone-mention", true))),
		fmt.Sprintf("Anti Here Mention: %s", antiStatusEmoji(settingBool(data, "anti-here-mention", true))),
		fmt.Sprintf("Anti Webhook: %s", antiStatusEmoji(settingBool(data, "anti-webhook-create", true))),
		fmt.Sprintf("Anti Guild Update: %s", antiStatusEmoji(settingBool(data, "anti-guild-update", true))),
		fmt.Sprintf("Anti Vanity Steal: %s", antiStatusEmoji(settingBool(data, "anti-vanity-steal", true))),
		fmt.Sprintf("Anti Member Role: %s", antiStatusEmoji(settingBool(data, "anti-member-role", true))),
	}

	_, _ = s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Title: "🛡️ PlayZ Security",
		Description: strings.Join([]string{
			"Anti-nuke protection overview for this server.",
			fmt.Sprintf("Master Protection: %s", antiStatusEmoji(settingBool(data, "antinuke-enabled", false))),
			"",
			"**Modules**",
			strings.Join(moduleLines, "\n"),
		}, "\n"),
		Fields: []*discordgo.MessageEmbedField{
			{Name: "⚙️ Enforcement", Value: fmt.Sprintf("Punishment: `%s`\nThreshold: `%s`", strings.ToUpper(mType), threshold), Inline: true},
			{Name: "📡 Logs", Value: logChannel, Inline: true},
			{Name: "🧾 Whitelist", Value: fmt.Sprintf("Users: `%d`", whitelistCount), Inline: true},
		},
		Color:  ThemeColor,
		Footer: &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Requested by: %s", m.Author.Username)},
	})
}

func (cmd *Commands) ClearWhitelists(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	if !canManageWhitelist(s, m) {
		sendError(s, m.ChannelID, "Only guild owner OR admin with role above bot can manage whitelists.", m.Author.Username)
		return
	}

	count, err := database.Database.ClearWhitelist(m.GuildID)
	if err != nil {
		sendError(s, m.ChannelID, err.Error(), m.Author.Username)
		return
	}
	sendSuccess(s, m.ChannelID, fmt.Sprintf("Removed `%d` whitelist entries for this server.", count), m.Author.Username)
}

func (cmd *Commands) LoggingChannel(s *discordgo.Session, message *discordgo.Message, ctx *Context) {
	if set, err := database.Database.SetData("$set", message.GuildID, "log-channel", message.ChannelID); !set {
		sendError(s, message.ChannelID, err.Error(), message.Author.Username)
		return
	}
	sendSuccess(s, message.ChannelID, "Set this channel as anti-nuke log channel.", message.Author.Username)
}

func (cmds *Commands) ModerationType(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	action := strings.ToLower(ctx.Fields[0])
	if !(action == "ban" || action == "kick") {
		sendError(s, m.ChannelID, "Use `ban` or `kick`.", m.Author.Username)
		return
	}

	if _, err := database.Database.SetData("$set", m.GuildID, "moderation-type", action); err != nil {
		sendError(s, m.ChannelID, err.Error(), m.Author.Username)
		return
	}

	sendSuccess(s, m.ChannelID, fmt.Sprintf("Moderation action set to `%s`.", action), m.Author.Username)
}

func (cmd *Commands) Prefix(s *discordgo.Session, message *discordgo.Message, ctx *Context) {
	newPrefix := strings.TrimSpace(ctx.Fields[0])
	if newPrefix == "" || len(newPrefix) > 5 || strings.Contains(newPrefix, " ") {
		sendError(s, message.ChannelID, "Prefix must be 1-5 chars and without spaces.", message.Author.Username)
		return
	}

	if set, err := database.Database.SetData("$set", message.GuildID, "prefix", newPrefix); !set {
		sendError(s, message.ChannelID, err.Error(), message.Author.Username)
		return
	}

	_, _ = s.ChannelMessageSendEmbed(message.ChannelID, &discordgo.MessageEmbed{
		Title:  fmt.Sprintf("Prefix has been set to `%s`", newPrefix),
		Footer: &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Requested by: %s", message.Author.Username)},
		Color:  ThemeColor,
	})
}

func (cmd *Commands) Settings(s *discordgo.Session, message *discordgo.Message, ctx *Context) {
	data, err := database.Database.FindData(message.GuildID)
	guild, _ := s.State.Guild(message.GuildID)

	if err != nil {
		sendError(s, message.ChannelID, err.Error(), message.Author.Username)
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:  fmt.Sprintf("%s current settings", guild.Name),
		Footer: &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Requested by: %s", message.Author.Username)},
		Color:  ThemeColor,
	}

	for index, value := range data {
		if utils.FindInSlice(blacklistedArgs, index) {
			continue
		}

		tempValue := fmt.Sprint(value)
		switch typed := value.(type) {
		case string:
			switch typed {
			case "on":
				tempValue = "Enabled"
			case "off":
				tempValue = "Disabled"
			case "nil":
				tempValue = "Not set"
			default:
				if index == "log-channel" {
					tempValue = fmt.Sprintf("<#%s>", typed)
				}
			}
		case bool:
			if typed {
				tempValue = "Enabled"
			} else {
				tempValue = "Disabled"
			}
		}

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: index, Value: tempValue, Inline: true})
	}
	_, _ = s.ChannelMessageSendEmbed(message.ChannelID, embed)
}

func (cmds *Commands) Threshold(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	value, err := strconv.Atoi(ctx.Fields[0])
	if err != nil {
		sendError(s, m.ChannelID, "Threshold must be a number between 1 and 10.", m.Author.Username)
		return
	}
	if value < 1 || value > 10 {
		sendError(s, m.ChannelID, "Threshold must be between 1 and 10.", m.Author.Username)
		return
	}

	if _, err := database.Database.SetData("$set", m.GuildID, "offense-threshold", fmt.Sprint(value)); err != nil {
		sendError(s, m.ChannelID, err.Error(), m.Author.Username)
		return
	}

	sendSuccess(s, m.ChannelID, fmt.Sprintf("Offense threshold set to `%d`.", value), m.Author.Username)
}

func (cmd *Commands) Toggle(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	module := strings.ToLower(ctx.Fields[0])
	if !utils.FindInSlice(validArgs, module) {
		sendError(s, m.ChannelID, "Invalid anti module name.", m.Author.Username)
		return
	}

	if len(ctx.Fields) < 2 {
		sendError(s, m.ChannelID, "Specify `on` or `off`.", m.Author.Username)
		return
	}

	var boolean bool
	switch strings.ToLower(ctx.Fields[1]) {
	case "on":
		boolean = true
	case "off":
		boolean = false
	default:
		sendError(s, m.ChannelID, "Use `on` or `off`.", m.Author.Username)
		return
	}

	if _, err := database.Database.SetToggle("$set", m.GuildID, module, boolean); err != nil {
		sendError(s, m.ChannelID, err.Error(), m.Author.Username)
		return
	}

	sendSuccess(s, m.ChannelID, fmt.Sprintf("Set `%s` to `%v`.", module, boolean), m.Author.Username)
}

func (cmd *Commands) Whitelist(s *discordgo.Session, message *discordgo.Message, ctx *Context) {
	if !canManageWhitelist(s, message) {
		sendError(s, message.ChannelID, "Only guild owner OR admin with role above bot can manage whitelists.", message.Author.Username)
		return
	}

	if whitelisted, err := database.Database.SetWhitelistData(message.GuildID, message.Mentions[0].ID, "$push", "users"); !whitelisted {
		sendError(s, message.ChannelID, err.Error(), message.Author.Username)
		return
	}
	sendSuccess(s, message.ChannelID, "🧾 Whitelisted that user.", message.Author.Username)
}

func (cmd *Commands) WhitelistInvite(s *discordgo.Session, message *discordgo.Message, ctx *Context) {
	if !canManageWhitelist(s, message) {
		sendError(s, message.ChannelID, "Only guild owner OR admin with role above bot can manage whitelists.", message.Author.Username)
		return
	}

	if whitelisted, err := database.Database.SetWhitelistData(message.GuildID, message.ChannelID, "$push", "whitelisted-invite-channels"); !whitelisted {
		sendError(s, message.ChannelID, err.Error(), message.Author.Username)
		return
	}
	sendSuccess(s, message.ChannelID, "🧾 Whitelisted this channel for sending discord.gg/ invites.", message.Author.Username)
}

func (cmd *Commands) WhitelistRole(s *discordgo.Session, message *discordgo.Message, ctx *Context) {
	if !canManageWhitelist(s, message) {
		sendError(s, message.ChannelID, "Only guild owner OR admin with role above bot can manage whitelists.", message.Author.Username)
		return
	}

	if whitelisted, err := database.Database.SetWhitelistData(message.GuildID, message.MentionRoles[0], "$push", "whitelisted-roles"); !whitelisted {
		sendError(s, message.ChannelID, err.Error(), message.Author.Username)
		return
	}
	sendSuccess(s, message.ChannelID, "🧾 Whitelisted that role.", message.Author.Username)
}

func (cmd *Commands) WhitelistWebhook(s *discordgo.Session, message *discordgo.Message, ctx *Context) {
	if !canManageWhitelist(s, message) {
		sendError(s, message.ChannelID, "Only guild owner OR admin with role above bot can manage whitelists.", message.Author.Username)
		return
	}

	if whitelisted, err := database.Database.SetWhitelistData(message.GuildID, message.ChannelID, "$push", "whitelisted-webhook-channels"); !whitelisted {
		sendError(s, message.ChannelID, err.Error(), message.Author.Username)
		return
	}
	sendSuccess(s, message.ChannelID, "🧾 Whitelisted this channel for webhook creation.", message.Author.Username)
}

func (cmd *Commands) Unwhitelist(s *discordgo.Session, message *discordgo.Message, ctx *Context) {
	if !canManageWhitelist(s, message) {
		sendError(s, message.ChannelID, "Only guild owner OR admin with role above bot can manage whitelists.", message.Author.Username)
		return
	}

	if whitelisted, err := database.Database.SetWhitelistData(message.GuildID, message.Mentions[0].ID, "$pull", "users"); !whitelisted {
		sendError(s, message.ChannelID, err.Error(), message.Author.Username)
		return
	}
	sendSuccess(s, message.ChannelID, "🗑️ Unwhitelisted that user.", message.Author.Username)
}

func (cmd *Commands) UnWhitelistInvite(s *discordgo.Session, message *discordgo.Message, ctx *Context) {
	if !canManageWhitelist(s, message) {
		sendError(s, message.ChannelID, "Only guild owner OR admin with role above bot can manage whitelists.", message.Author.Username)
		return
	}

	if whitelisted, err := database.Database.SetWhitelistData(message.GuildID, message.ChannelID, "$pull", "whitelisted-invite-channels"); !whitelisted {
		sendError(s, message.ChannelID, err.Error(), message.Author.Username)
		return
	}
	sendSuccess(s, message.ChannelID, "🗑️ Removed invite whitelist on this channel.", message.Author.Username)
}

func (cmd *Commands) UnWhitelistRole(s *discordgo.Session, message *discordgo.Message, ctx *Context) {
	if !canManageWhitelist(s, message) {
		sendError(s, message.ChannelID, "Only guild owner OR admin with role above bot can manage whitelists.", message.Author.Username)
		return
	}

	if whitelisted, err := database.Database.SetWhitelistData(message.GuildID, message.MentionRoles[0], "$pull", "whitelisted-roles"); !whitelisted {
		sendError(s, message.ChannelID, err.Error(), message.Author.Username)
		return
	}
	sendSuccess(s, message.ChannelID, "🗑️ Unwhitelisted that role.", message.Author.Username)
}

func (cmd *Commands) UnWhitelistWebhook(s *discordgo.Session, message *discordgo.Message, ctx *Context) {
	if !canManageWhitelist(s, message) {
		sendError(s, message.ChannelID, "Only guild owner OR admin with role above bot can manage whitelists.", message.Author.Username)
		return
	}

	if whitelisted, err := database.Database.SetWhitelistData(message.GuildID, message.ChannelID, "$pull", "whitelisted-webhook-channels"); !whitelisted {
		sendError(s, message.ChannelID, err.Error(), message.Author.Username)
		return
	}
	sendSuccess(s, message.ChannelID, "🗑️ Removed webhook whitelist on this channel.", message.Author.Username)
}

func (cmd *Commands) ViewWhitelisted(s *discordgo.Session, message *discordgo.Message, ctx *Context) {
	data, err := database.Database.FindData(message.GuildID)
	if err != nil {
		_, _ = s.ChannelMessageSend(message.ChannelID, err.Error())
		return
	}

	whitelistedUsers := make([]string, 0)

	if users, ok := data["users"].([]string); ok {
		for _, userID := range users {
			member, memberErr := s.State.Member(message.GuildID, userID)
			if memberErr != nil {
				member, memberErr = s.GuildMember(message.GuildID, userID)
			}
			if memberErr != nil || member == nil || member.User == nil {
				whitelistedUsers = append(whitelistedUsers, fmt.Sprintf("- User ID: `%s`", userID))
				continue
			}
			whitelistedUsers = append(whitelistedUsers, fmt.Sprintf("- User: %s#%s", member.User.Username, member.User.Discriminator))
		}
	} else if users, ok := data["users"].([]interface{}); ok {
		for _, raw := range users {
			userID, idOK := raw.(string)
			if !idOK {
				continue
			}
			member, memberErr := s.State.Member(message.GuildID, userID)
			if memberErr != nil {
				member, memberErr = s.GuildMember(message.GuildID, userID)
			}
			if memberErr != nil || member == nil || member.User == nil {
				whitelistedUsers = append(whitelistedUsers, fmt.Sprintf("- User ID: `%s`", userID))
				continue
			}
			whitelistedUsers = append(whitelistedUsers, fmt.Sprintf("- User: %s#%s", member.User.Username, member.User.Discriminator))
		}
	}

	if roles, ok := data["whitelisted-roles"].([]string); ok {
		for _, roleID := range roles {
			whitelistedUsers = append(whitelistedUsers, fmt.Sprintf("- Role: <@&%s>", roleID))
		}
	} else if roles, ok := data["whitelisted-roles"].([]interface{}); ok {
		for _, raw := range roles {
			roleID, idOK := raw.(string)
			if !idOK {
				continue
			}
			whitelistedUsers = append(whitelistedUsers, fmt.Sprintf("- Role: <@&%s>", roleID))
		}
	}

	if channels, ok := data["whitelisted-invite-channels"].([]string); ok {
		for _, inviteID := range channels {
			whitelistedUsers = append(whitelistedUsers, fmt.Sprintf("- Invite channel: <#%s>", inviteID))
		}
	} else if channels, ok := data["whitelisted-invite-channels"].([]interface{}); ok {
		for _, raw := range channels {
			inviteID, idOK := raw.(string)
			if !idOK {
				continue
			}
			whitelistedUsers = append(whitelistedUsers, fmt.Sprintf("- Invite channel: <#%s>", inviteID))
		}
	}

	if channels, ok := data["whitelisted-webhook-channels"].([]string); ok {
		for _, webhookID := range channels {
			whitelistedUsers = append(whitelistedUsers, fmt.Sprintf("- Webhook channel: <#%s>", webhookID))
		}
	} else if channels, ok := data["whitelisted-webhook-channels"].([]interface{}); ok {
		for _, raw := range channels {
			webhookID, idOK := raw.(string)
			if !idOK {
				continue
			}
			whitelistedUsers = append(whitelistedUsers, fmt.Sprintf("- Webhook channel: <#%s>", webhookID))
		}
	}

	if len(whitelistedUsers) == 0 {
		whitelistedUsers = append(whitelistedUsers, "No whitelist entries found.")
	}

	_, _ = s.ChannelMessageSendEmbed(message.ChannelID, &discordgo.MessageEmbed{
		Title:       "📜 Whitelisted Data",
		Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Requested by: %s", message.Author.Username)},
		Description: strings.Join(whitelistedUsers, "\n"),
		Color:       ThemeColor,
	})
}

var (
	blacklistedArgs = []string{
		"users",
		"guild-id",
		"guild-name",
		"vanity-url",
		"whitelisted-roles",
		"whitelisted-invite-channels",
		"whitelisted-webhook-channels",
	}

	validArgs = []string{
		"anti-everyone-mention",
		"anti-here-mention",
		"anti-ban",
		"anti-bot",
		"anti-kick",
		"anti-prune",
		"anti-guild-update",
		"anti-name-change",
		"anti-widget-spam",
		"anti-member-role",
		"anti-role-create",
		"anti-role-delete",
		"anti-role-update",
		"anti-vanity-steal",
		"anti-channel-create",
		"anti-channel-delete",
		"anti-channel-update",
		"anti-webhook-create",
	}
)

func canManageAntiNuke(s *discordgo.Session, m *discordgo.Message) bool {
	if utils.GetGuildOwner(s, m.GuildID) == m.Author.ID {
		return true
	}

	if !utils.HasPerms(s, m, m.GuildID, m.Author.ID, discordgo.PermissionAdministrator) {
		return false
	}

	botMember, err := s.State.Member(m.GuildID, s.State.User.ID)
	if err != nil {
		botMember, err = s.GuildMember(m.GuildID, s.State.User.ID)
		if err != nil {
			return false
		}
	}

	var authorMember *discordgo.Member
	if m.Member != nil {
		authorMember = m.Member
	} else {
		authorMember, err = s.GuildMember(m.GuildID, m.Author.ID)
		if err != nil {
			return false
		}
	}

	authorRole := utils.HighestRole(s, m.GuildID, authorMember)
	botRole := utils.HighestRole(s, m.GuildID, botMember)
	if authorRole == nil || botRole == nil {
		return false
	}

	return utils.IsAbove(authorRole, botRole)
}

func antiStatusEmoji(enabled bool) string {
	if enabled {
		return "<:antinuke_enable:1489191815214272684>"
	}
	return "<:evo_antinuke_disable:1489191890778849280>"
}

func settingBool(data map[string]interface{}, key string, fallback bool) bool {
	raw, ok := data[key]
	if !ok {
		return fallback
	}
	val, ok := raw.(bool)
	if !ok {
		return fallback
	}
	return val
}

func canManageWhitelist(s *discordgo.Session, m *discordgo.Message) bool {
	if utils.GetGuildOwner(s, m.GuildID) == m.Author.ID {
		return true
	}

	if !utils.HasPerms(s, m, m.GuildID, m.Author.ID, discordgo.PermissionAdministrator) {
		return false
	}

	botMember, err := s.State.Member(m.GuildID, s.State.User.ID)
	if err != nil {
		botMember, err = s.GuildMember(m.GuildID, s.State.User.ID)
		if err != nil {
			return false
		}
	}

	var authorMember *discordgo.Member
	if m.Member != nil {
		authorMember = m.Member
	} else {
		authorMember, err = s.GuildMember(m.GuildID, m.Author.ID)
		if err != nil {
			return false
		}
	}

	authorRole := utils.HighestRole(s, m.GuildID, authorMember)
	botRole := utils.HighestRole(s, m.GuildID, botMember)
	if authorRole == nil || botRole == nil {
		return false
	}

	return utils.IsAbove(authorRole, botRole)
}
