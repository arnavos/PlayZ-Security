package commands

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/summrs-dev-team/summrs-premium/utils"
)

const (
	massUnbanYesPrefix = "massunban:yes:"
	massUnbanNoPrefix  = "massunban:no:"
)

func (cmd *Commands) Ban(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	member, ok := validateModerationTarget(s, m, m.Mentions[0].ID, discordgo.PermissionBanMembers, "ban")
	if !ok {
		return
	}

	reason := moderationReason(ctx)
	_ = sendModerationDM(s, member.User.ID, "Banned", m.GuildID, reason)

	err := s.GuildBanCreateWithReason(m.GuildID, member.User.ID, moderationAuditReason(m, "ban", reason), 0)
	if err != nil {
		sendError(s, m.ChannelID, fmt.Sprintf("Could not ban <@%s>.", member.User.ID), m.Author.Username)
		return
	}

	sendModerationResult(s, m.ChannelID, "Ban", member.User.ID, m.Author.Username, reason)
}

func (cmd *Commands) Kick(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	member, ok := validateModerationTarget(s, m, m.Mentions[0].ID, discordgo.PermissionKickMembers, "kick")
	if !ok {
		return
	}

	reason := moderationReason(ctx)
	_ = sendModerationDM(s, member.User.ID, "Kicked", m.GuildID, reason)

	err := s.GuildMemberDeleteWithReason(m.GuildID, member.User.ID, moderationAuditReason(m, "kick", reason))
	if err != nil {
		sendError(s, m.ChannelID, fmt.Sprintf("Could not kick <@%s>.", member.User.ID), m.Author.Username)
		return
	}
	sendModerationResult(s, m.ChannelID, "Kick", member.User.ID, m.Author.Username, reason)
}

func (cmd *Commands) Lockdown(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	err := s.ChannelPermissionSet(m.ChannelID, m.GuildID, discordgo.PermissionOverwriteTypeRole, 0, discordgo.PermissionSendMessages)
	if err != nil {
		sendError(s, m.ChannelID, "Couldn't lock this channel. Check my permissions.", m.Author.Username)
		return
	}
	_, _ = s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("🔒 Successfully locked <#%s>.", m.ChannelID),
		Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Locked by: %s", m.Author.Username)},
		Color:       ThemeColor,
	})
}

func (cmd *Commands) Hide(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	err := s.ChannelPermissionSet(
		m.ChannelID,
		m.GuildID,
		discordgo.PermissionOverwriteTypeRole,
		0,
		discordgo.PermissionViewChannel,
	)
	if err != nil {
		sendError(s, m.ChannelID, "Couldn't hide this channel. Check my permissions.", m.Author.Username)
		return
	}
	sendSuccess(s, m.ChannelID, fmt.Sprintf("Successfully hid <#%s>.", m.ChannelID), m.Author.Username)
}

func (cmd *Commands) SlowMode(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	seconds, err := strconv.Atoi(ctx.Fields[0])
	if err != nil {
		sendError(s, m.ChannelID, "You must specify a number of seconds (0-21600).", m.Author.Username)
		return
	}
	if seconds < 0 || seconds > 21600 {
		sendError(s, m.ChannelID, "Slowmode must be between 0 and 21600 seconds.", m.Author.Username)
		return
	}
	secondsVal := seconds

	channel, err := s.ChannelEditComplex(m.ChannelID, &discordgo.ChannelEdit{RateLimitPerUser: &secondsVal})
	if err != nil {
		sendError(s, m.ChannelID, "Couldn't set slowmode on this channel. Check my permissions.", m.Author.Username)
		return
	}
	_, _ = s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("Set <#%s> slowmode to %d seconds", channel.ID, seconds),
		Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Slowmode set by: %s", m.Author.Username)},
		Color:       ThemeColor,
	})
}

func (cmd *Commands) Unban(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	bans, err := s.GuildBans(m.GuildID, 1000, "", "")
	if err != nil {
		sendError(s, m.ChannelID, "Could not fetch the guild bans.", m.Author.Username)
		return
	}

	candidateCount, protectedCount := massUnbanBreakdown(bans)
	if candidateCount == 0 {
		_, _ = s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
			Title:       "🔓 Mass Unban",
			Description: fmt.Sprintf("No users to unban. Kept %d %s protected bans.", protectedCount, utils.ReasonPrefix),
			Color:       ThemeColor,
			Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Requested by: %s", m.Author.Username)},
		})
		return
	}

	yesID := massUnbanYesPrefix + m.Author.ID
	noID := massUnbanNoPrefix + m.Author.ID

	_, _ = s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Embed: &discordgo.MessageEmbed{
			Title:       "⚠️ Mass Unban Confirmation",
			Description: fmt.Sprintf("Are you sure you want to unban `%d` users?\nProtected `%s` bans: `%d`", candidateCount, utils.ReasonPrefix, protectedCount),
			Color:       ThemeColor,
			Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Requested by: %s", m.Author.Username)},
		},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Style:    discordgo.SuccessButton,
						Label:    "✅ Yes",
						CustomID: yesID,
					},
					discordgo.Button{
						Style:    discordgo.DangerButton,
						Label:    "❌ No",
						CustomID: noID,
					},
				},
			},
		},
	})
}

func massUnbanBreakdown(bans []*discordgo.GuildBan) (candidateCount int, protectedCount int) {
	for _, ban := range bans {
		if strings.Contains(strings.ToLower(ban.Reason), strings.ToLower(utils.ReasonPrefix)) {
			protectedCount++
			continue
		}
		candidateCount++
	}
	return candidateCount, protectedCount
}

func runMassUnban(s *discordgo.Session, guildID string) (unbanned int, protected int, err error) {
	bans, err := s.GuildBans(guildID, 1000, "", "")
	if err != nil {
		return 0, 0, err
	}

	for _, ban := range bans {
		if strings.Contains(strings.ToLower(ban.Reason), strings.ToLower(utils.ReasonPrefix)) {
			protected++
			continue
		}
		if deleteErr := s.GuildBanDelete(guildID, ban.User.ID); deleteErr == nil {
			unbanned++
		}
	}

	return unbanned, protected, nil
}

func (cmd *Commands) UnLockdown(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	err := s.ChannelPermissionSet(m.ChannelID, m.GuildID, discordgo.PermissionOverwriteTypeRole, discordgo.PermissionSendMessages, 0)
	if err != nil {
		sendError(s, m.ChannelID, "Couldn't unlock this channel. Check my permissions.", m.Author.Username)
		return
	}
	_, _ = s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("🔓 Successfully unlocked <#%s>.", m.ChannelID),
		Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Unlocked by: %s", m.Author.Username)},
		Color:       ThemeColor,
	})
}

func (cmd *Commands) Unhide(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	err := s.ChannelPermissionSet(
		m.ChannelID,
		m.GuildID,
		discordgo.PermissionOverwriteTypeRole,
		discordgo.PermissionViewChannel,
		0,
	)
	if err != nil {
		sendError(s, m.ChannelID, "Couldn't unhide this channel. Check my permissions.", m.Author.Username)
		return
	}
	sendSuccess(s, m.ChannelID, fmt.Sprintf("Successfully unhid <#%s>.", m.ChannelID), m.Author.Username)
}

func (cmd *Commands) UnSlowMode(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	zero := 0
	channel, err := s.ChannelEditComplex(m.ChannelID, &discordgo.ChannelEdit{RateLimitPerUser: &zero})
	if err != nil {
		sendError(s, m.ChannelID, "Couldn't disable slowmode. Check my permissions.", m.Author.Username)
		return
	}
	_, _ = s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("⏱️ Successfully disabled slowmode for <#%s>.", channel.ID),
		Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Slowmode turned off by: %s", m.Author.Username)},
		Color:       ThemeColor,
	})
}

func validateModerationTarget(s *discordgo.Session, m *discordgo.Message, targetID string, requiredPerm int, action string) (*discordgo.Member, bool) {
	member, err := s.GuildMember(m.GuildID, targetID)
	if err != nil {
		sendError(s, m.ChannelID, "Couldn't fetch that member.", m.Author.Username)
		return nil, false
	}

	if targetID == m.Author.ID {
		sendError(s, m.ChannelID, "You cannot "+action+" yourself.", m.Author.Username)
		return nil, false
	}
	if targetID == s.State.User.ID {
		sendError(s, m.ChannelID, "You cannot "+action+" this bot.", m.Author.Username)
		return nil, false
	}
	if targetID == utils.GetGuildOwner(s, m.GuildID) {
		sendError(s, m.ChannelID, "You cannot "+action+" the server owner.", m.Author.Username)
		return nil, false
	}

	if !utils.HasPerms(s, nil, m.GuildID, s.State.User.ID, requiredPerm) {
		sendError(s, m.ChannelID, "I don't have required permissions to "+action+" this member.", m.Author.Username)
		return nil, false
	}

	botMember, err := s.State.Member(m.GuildID, s.State.User.ID)
	if err != nil {
		botMember, err = s.GuildMember(m.GuildID, s.State.User.ID)
		if err != nil {
			sendError(s, m.ChannelID, "Couldn't verify bot role hierarchy.", m.Author.Username)
			return nil, false
		}
	}

	userRole := utils.HighestRole(s, m.GuildID, m.Member)
	targetRole := utils.HighestRole(s, m.GuildID, member)
	botRole := utils.HighestRole(s, m.GuildID, botMember)
	if userRole == nil || targetRole == nil || botRole == nil {
		sendError(s, m.ChannelID, "Role hierarchy check failed.", m.Author.Username)
		return nil, false
	}

	if !utils.IsAbove(userRole, targetRole) && m.Author.ID != utils.GetGuildOwner(s, m.GuildID) {
		sendError(s, m.ChannelID, "Your role must be above the target member.", m.Author.Username)
		return nil, false
	}
	if !utils.IsAbove(botRole, targetRole) {
		sendError(s, m.ChannelID, "My role must be above the target member.", m.Author.Username)
		return nil, false
	}

	return member, true
}

func sendModerationResult(s *discordgo.Session, channelID, action, targetID, moderator, reason string) {
	emoji := "✅"
	verb := strings.ToLower(action) + "ed"
	if strings.EqualFold(action, "Kick") {
		verb = "kicked"
	}

	_, _ = s.ChannelMessageSendEmbed(channelID, &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s %s Executed", emoji, action),
		Description: fmt.Sprintf("Successfully `%s` <@%s>.", verb, targetID),
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Action", Value: action, Inline: true},
			{Name: "Target", Value: fmt.Sprintf("<@%s>", targetID), Inline: true},
			{Name: "Moderator", Value: moderator, Inline: true},
			{Name: "Reason", Value: reason, Inline: false},
		},
		Color:     ThemeColor,
		Timestamp: time.Now().Format(time.RFC3339),
		Footer:    &discordgo.MessageEmbedFooter{Text: "PlayZ Security Moderation"},
	})
}

func moderationReason(ctx *Context) string {
	if len(ctx.Fields) <= 1 {
		return "Reason not provided"
	}

	reason := strings.TrimSpace(strings.Join(ctx.Fields[1:], " "))
	if reason == "" {
		return "Reason not provided"
	}

	return reason
}

func sendModerationDM(s *discordgo.Session, userID, action, guildID, reason string) error {
	dm, err := s.UserChannelCreate(userID)
	if err != nil {
		return err
	}

	_, err = s.ChannelMessageSendEmbed(dm.ID, &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🚨 You have been %s", strings.ToLower(action)),
		Description: fmt.Sprintf("Your account was %s in guild `%s`.", strings.ToLower(action), guildID),
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Reason", Value: reason},
		},
		Color:     ThemeColor,
		Timestamp: time.Now().Format(time.RFC3339),
		Footer:    &discordgo.MessageEmbedFooter{Text: "PlayZ Security Moderation"},
	})
	return err
}

func moderationAuditReason(m *discordgo.Message, action, reason string) string {
	details := fmt.Sprintf(
		"command %s | By: %s#%s (%s) | Reason: %s",
		strings.ToLower(action),
		m.Author.Username,
		m.Author.Discriminator,
		m.Author.ID,
		reason,
	)
	full := utils.FormatReason(details)
	if len(full) > 512 {
		return full[:512]
	}
	return full
}
