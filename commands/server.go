package commands

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func (cmd *Commands) MemberCount(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	guild, err := s.State.Guild(m.GuildID)
	if err != nil {
		return
	}

	s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Title: fmt.Sprintf("👥 %s", guild.Name),
		Fields: []*discordgo.MessageEmbedField{
			{Name: "📊 Members", Value: fmt.Sprint(guild.MemberCount), Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Requested by: %s", m.Author.Username)},
		Color:  ThemeColor,
	})

}

func (cmd *Commands) Nuke(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	channel, err := s.Channel(m.ChannelID)
	if err != nil {
		return
	}

	_, err = s.ChannelDelete(channel.ID)
	if err != nil {
		return
	}

	channel, err = s.GuildChannelCreateComplex(m.GuildID, discordgo.GuildChannelCreateData{
		Name:                 channel.Name,
		Type:                 channel.Type,
		Topic:                channel.Topic,
		RateLimitPerUser:     channel.RateLimitPerUser,
		Position:             channel.Position,
		PermissionOverwrites: channel.PermissionOverwrites,
		ParentID:             channel.ParentID,
		NSFW:                 channel.NSFW,
	})
	if err != nil {
		return
	}

	s.ChannelMessageSendEmbed(channel.ID, &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{Name: fmt.Sprintf("💣 Channel has been nuked by %s#%s", m.Author.Username, m.Author.Discriminator)},
		Image:  &discordgo.MessageEmbedImage{URL: "https://media2.giphy.com/media/HhTXt43pk1I1W/giphy.gif"},
		Color:  ThemeColor,
	})

}

func (cmd *Commands) ServerBanner(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	guild, err := s.State.Guild(m.GuildID)
	if err != nil {
		return
	}

	if len(guild.Banner) == 0 {
		s.ChannelMessageSend(m.ChannelID, "🖼️ There is no guild banner.")
		return
	}

	s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Title: fmt.Sprintf("🖼️ %s's server banner", guild.Name),
		Image: &discordgo.MessageEmbedImage{URL: discordgo.EndpointGuildBanner(guild.ID, guild.Banner)},

		Footer: &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Requested by: %s", m.Author.Username)},
		Color:  ThemeColor,
	})

}

func (cmd *Commands) ServerIcon(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	guild, err := s.State.Guild(m.GuildID)
	if err != nil {
		return
	}

	if len(guild.IconURL("1024")) == 0 {
		s.ChannelMessageSend(m.ChannelID, "🧿 There is no guild icon.")
		return
	}

	s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Title: fmt.Sprintf("🧿 %s's server icon", guild.Name),
		Image: &discordgo.MessageEmbedImage{
			URL: guild.IconURL("1024"),
		},
		Footer: &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Requested by: %s", m.Author.Username)},
		Color:  ThemeColor,
	})

}

func (cmd *Commands) ServerInfo(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	guild, err := s.State.Guild(m.GuildID)
	if err != nil {
		return
	}

	guildTime, _ := discordgo.SnowflakeTimestamp(guild.ID)
	textChannels := 0
	voiceChannels := 0
	for _, ch := range guild.Channels {
		switch ch.Type {
		case discordgo.ChannelTypeGuildText, discordgo.ChannelTypeGuildNews, discordgo.ChannelTypeGuildForum:
			textChannels++
		case discordgo.ChannelTypeGuildVoice, discordgo.ChannelTypeGuildStageVoice:
			voiceChannels++
		}
	}

	activeVoiceUsers := 0
	activeVoiceChannels := map[string]struct{}{}
	for _, vs := range guild.VoiceStates {
		if vs.ChannelID == "" {
			continue
		}
		activeVoiceUsers++
		activeVoiceChannels[vs.ChannelID] = struct{}{}
	}

	systemChannel := "None"
	if guild.SystemChannelID != "" {
		systemChannel = fmt.Sprintf("<#%s>", guild.SystemChannelID)
	}

	verification := "None"
	switch guild.VerificationLevel {
	case discordgo.VerificationLevelLow:
		verification = "Low"
	case discordgo.VerificationLevelMedium:
		verification = "Medium"
	case discordgo.VerificationLevelHigh:
		verification = "High"
	case discordgo.VerificationLevelVeryHigh:
		verification = "Very High"
	}

	boostLevel := "0"
	switch guild.PremiumTier {
	case discordgo.PremiumTier1:
		boostLevel = "1"
	case discordgo.PremiumTier2:
		boostLevel = "2"
	case discordgo.PremiumTier3:
		boostLevel = "3"
	}

	description := strings.Join([]string{
		fmt.Sprintf("**Server:** `%s`", guild.Name),
		fmt.Sprintf("<:owner:1489205008535650388> **Owner:** <@%s>", guild.OwnerID),
		fmt.Sprintf("📅 **Created:** `%s`", guildTime.Format("02 Jan 2006")),
		fmt.Sprintf("🆔 **Guild ID:** `%s`", guild.ID),
		"",
		"**Statistics**",
		fmt.Sprintf("👥 Members: `%d`", guild.MemberCount),
		fmt.Sprintf("💬 Text Channels: `%d`", textChannels),
		fmt.Sprintf("🔊 Voice Channels: `%d`", voiceChannels),
		fmt.Sprintf("🛡️ Roles: `%d`", len(guild.Roles)),
		"",
		"**Voice Activity**",
		fmt.Sprintf("🎙️ Active Users: `%d`", activeVoiceUsers),
		fmt.Sprintf("📡 Active Channels: `%d`", len(activeVoiceChannels)),
		"",
		"**Security & Boost**",
		fmt.Sprintf("✅ Verification: `%s`", verification),
		fmt.Sprintf("🚀 Boost Level: `%s`", boostLevel),
		fmt.Sprintf("💎 Boost Count: `%d`", guild.PremiumSubscriptionCount),
		fmt.Sprintf("📣 System Channel: %s", systemChannel),
	}, "\n")

	s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s Info", guild.Name),
		Description: description,
		Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: guild.IconURL("1024")},
		Color:       ThemeColor,
		Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Requested by: %s", m.Author.Username)},
	})

}
