package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/summrs-dev-team/summrs-premium/database"
	"github.com/summrs-dev-team/summrs-premium/utils"
)

const helpMenuCustomID = "playz_help_menu"
const helpDropdownTTL = 30 * time.Second

func (cmd *Commands) Help(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	category := "home"
	if len(ctx.Fields) > 0 {
		category = normalizeCategory(ctx.Fields[0])
	}

	embed := buildHelpEmbed(s, m.Author.Username, ctx.Prefix, category)
	components := buildHelpComponents(category, m.Author.ID)

	sent, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Embed:      embed,
		Components: components,
	})
	if err != nil || sent == nil {
		return
	}

	go expireHelpDropdown(s, sent.ChannelID, sent.ID, helpDropdownTTL)
}

func (cmd *Commands) InteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionMessageComponent {
		return
	}

	data := i.MessageComponentData()

	if strings.HasPrefix(data.CustomID, massUnbanYesPrefix) || strings.HasPrefix(data.CustomID, massUnbanNoPrefix) {
		cmd.handleMassUnbanInteraction(s, i, data.CustomID)
		return
	}

	if !strings.HasPrefix(data.CustomID, helpMenuCustomID+":") || len(data.Values) == 0 {
		return
	}

	authorID := strings.TrimPrefix(data.CustomID, helpMenuCustomID+":")
	clickerID := ""
	if i.Member != nil && i.Member.User != nil {
		clickerID = i.Member.User.ID
	} else if i.User != nil {
		clickerID = i.User.ID
	}
	if clickerID == "" || clickerID != authorID {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "It's not your interaction.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	category := normalizeCategory(data.Values[0])
	prefix := ">"
	if i.GuildID != "" {
		if row, err := database.Database.FindData(i.GuildID); err == nil {
			if p, ok := row["prefix"].(string); ok && p != "" {
				prefix = p
			}
		}
	}

	requester := "Unknown"
	if i.Member != nil && i.Member.User != nil {
		requester = i.Member.User.Username
	} else if i.User != nil {
		requester = i.User.Username
	}

	embed := buildHelpEmbed(s, requester, prefix, category)
	components := buildHelpComponents(category, authorID)

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func expireHelpDropdown(s *discordgo.Session, channelID, messageID string, ttl time.Duration) {
	time.Sleep(ttl)
	empty := []discordgo.MessageComponent{}
	edit := discordgo.NewMessageEdit(channelID, messageID)
	edit.Components = &empty
	_, _ = s.ChannelMessageEditComplex(edit)
}

func (cmd *Commands) handleMassUnbanInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, customID string) {
	var actorID string
	confirm := false

	switch {
	case strings.HasPrefix(customID, massUnbanYesPrefix):
		actorID = strings.TrimPrefix(customID, massUnbanYesPrefix)
		confirm = true
	case strings.HasPrefix(customID, massUnbanNoPrefix):
		actorID = strings.TrimPrefix(customID, massUnbanNoPrefix)
	default:
		return
	}

	clickerID := ""
	if i.Member != nil && i.Member.User != nil {
		clickerID = i.Member.User.ID
	} else if i.User != nil {
		clickerID = i.User.ID
	}

	if clickerID == "" || clickerID != actorID {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Only the command author can use these buttons.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if !confirm {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{{
					Title:       "Mass Unban Cancelled",
					Description: "No users were unbanned.",
					Color:       ThemeColor,
				}},
				Components: []discordgo.MessageComponent{},
			},
		})
		return
	}

	unbanned, protected, err := runMassUnban(s, i.GuildID)
	if err != nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{{
					Title:       "Mass Unban Failed",
					Description: "Could not fetch bans for this guild.",
					Color:       ThemeColor,
				}},
				Components: []discordgo.MessageComponent{},
			},
		})
		return
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{{
				Title:       "Mass Unban Complete",
				Description: fmt.Sprintf("Unbanned `%d` users | Kept `%d` %s bans.", unbanned, protected, utils.ReasonPrefix),
				Color:       ThemeColor,
			}},
			Components: []discordgo.MessageComponent{},
		},
	})
}

func normalizeCategory(category string) string {
	c := strings.ToLower(strings.TrimSpace(category))
	switch c {
	case "home", "main":
		return "home"
	case "information", "info":
		return "information"
	case "anti", "antinuke", "anti-nuke":
		return "anti"
	case "moderation", "mod":
		return "moderation"
	case "settings", "config":
		return "settings"
	default:
		return "home"
	}
}

func buildHelpComponents(selected string, authorID string) []discordgo.MessageComponent {
	opts := []discordgo.SelectMenuOption{
		{Label: "Home", Value: "home", Description: "Main command center", Emoji: &discordgo.ComponentEmoji{Name: "home", ID: "1488971262729392288"}, Default: selected == "home"},
		{Label: "Information", Value: "information", Description: "Utility and info commands", Emoji: &discordgo.ComponentEmoji{Name: "info", ID: "1488967515999568102"}, Default: selected == "information"},
		{Label: "AntiNuke", Value: "anti", Description: "Security controls and whitelist", Emoji: &discordgo.ComponentEmoji{Name: "antinuke", ID: "1488969178630324246"}, Default: selected == "anti"},
		{Label: "Moderation", Value: "moderation", Description: "Ban/kick/lock/slowmode tools", Emoji: &discordgo.ComponentEmoji{Name: "mod", ID: "1488967409640542269"}, Default: selected == "moderation"},
		{Label: "Settings", Value: "settings", Description: "Prefix, logs, base settings", Emoji: &discordgo.ComponentEmoji{Name: "settings", ID: "1488969685994049706"}, Default: selected == "settings"},
	}

	min := 1
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{
				CustomID:    helpMenuCustomID + ":" + authorID,
				Placeholder: "Choose a help category",
				MinValues:   &min,
				MaxValues:   1,
				Options:     opts,
			},
		}},
	}
}

func buildHelpEmbed(s *discordgo.Session, requester, prefix, category string) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Color:     ThemeColor,
		Thumbnail: &discordgo.MessageEmbedThumbnail{URL: s.State.User.AvatarURL("256")},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Requested by: %s", requester),
		},
	}

	switch category {
	case "information":
		embed.Title = "<:info:1488967515999568102> Information Commands"
		embed.Description = strings.Join([]string{
			fmt.Sprintf("`%sserverinfo` `%sbotinfo` `%smembercount`", prefix, prefix, prefix),
			fmt.Sprintf("`%suserinfo @user` `%savatar [@user]` `%sping`", prefix, prefix, prefix),
		}, "\n")
	case "anti":
		embed.Title = "<:antinuke:1488969178630324246> AntiNuke Commands"
		embed.Description = strings.Join([]string{
			fmt.Sprintf("`%santinuke <on/off>`", prefix),
			fmt.Sprintf("`%stoggle <module> <on/off>`", prefix),
			fmt.Sprintf("`%smoderationtype <ban/kick>` `%sthreshold <count>`", prefix, prefix),
			fmt.Sprintf("`%swhitelist @user` `%sunwhitelist @user` `%swhitelisted`", prefix, prefix, prefix),
		}, "\n")
		embed.Fields = []*discordgo.MessageEmbedField{{
			Name:  "Hot Modules",
			Value: "`anti-prune` `anti-everyone-mention` `anti-channel-update` `anti-guild-update`",
		}}
	case "moderation":
		embed.Title = "<:mod:1488967409640542269> Moderation Commands"
		embed.Description = strings.Join([]string{
			fmt.Sprintf("`%sban @user` `%skick @user` `%smassunban`", prefix, prefix, prefix),
			fmt.Sprintf("`%slock` `%sunlock` `%shide` `%sunhide`", prefix, prefix, prefix, prefix),
			fmt.Sprintf("`%sslowmode <sec>` `%sunslowmode`", prefix, prefix),
			fmt.Sprintf("`%snuke`", prefix),
		}, "\n")
	case "settings":
		embed.Title = "<:settings:1488969685994049706> Settings Commands"
		embed.Description = strings.Join([]string{
			fmt.Sprintf("`%sprefix <new>` `%slogchannel`", prefix, prefix),
			fmt.Sprintf("`%santiinvite <on/off>` `%ssettings`", prefix, prefix),
		}, "\n")
	default:
		embed.Title = "PlayZ Security Help"
		embed.Description = strings.Join([]string{
			fmt.Sprintf("Prefix: `%s`", prefix),
			"Use the dropdown below to browse command categories.",
			"Select a category to see exact available commands.",
		}, "\n")
		embed.Fields = []*discordgo.MessageEmbedField{
			{Name: "<:info:1488967515999568102> Information", Value: "Bot info, user/server info, avatar, ping", Inline: true},
			{Name: "<:antinuke:1488969178630324246> Anti-Nuke", Value: "Master toggle, module toggles, whitelist controls", Inline: true},
			{Name: "<:mod:1488967409640542269> Moderation", Value: "Ban, kick, lock/unlock, slowmode, massunban", Inline: true},
			{Name: "<:settings:1488969685994049706> Settings", Value: "Prefix, antiinvite, log channel, settings view", Inline: true},
		}
	}

	return embed
}
