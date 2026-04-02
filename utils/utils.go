package utils

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/summrs-dev-team/summrs-premium/database"
)

const ReasonPrefix = "PlayZ Security"
const AntiNukeLogColor = 0xA855F7

type AntiNukeLogData struct {
	EventTitle     string
	CriminalID     string
	Crime          string
	VictimID       string
	CounterAction  string
	ActionDuration string
}

type RecoveryFunc func(*discordgo.AuditLogEntry) error

type offenseRecord struct {
	Count int
	Last  time.Time
}

var (
	auditCacheMu   sync.Mutex
	auditSeen      = make(map[string]time.Time)
	offenseCache   = make(map[string]offenseRecord)
	offenseWindow  = 12 * time.Second
	auditTTLWindow = 2 * time.Minute
)

func FormatReason(reason string) string {
	return fmt.Sprintf("%s | %s", ReasonPrefix, reason)
}

func FormatActionLatency(started time.Time, auditEntryID string) string {
	processSeconds := time.Since(started).Seconds()
	if processSeconds < 0 {
		processSeconds = 0
	}

	totalSeconds := processSeconds
	if auditEntryID != "" {
		if entryTime, err := discordgo.SnowflakeTimestamp(auditEntryID); err == nil {
			candidate := time.Since(entryTime).Seconds()
			// Ignore impossible/old values and prefer the slower of processing vs audit age.
			if candidate >= 0 && candidate <= 30 {
				if candidate > totalSeconds {
					totalSeconds = candidate
				}
			}
		}
	}

	return fmt.Sprintf("%.2fs", totalSeconds)
}

func FindAudit(s *discordgo.Session, guildID string, auditType int) (*discordgo.AuditLogEntry, interface{}, error) {
	if !HasPerms(s, nil, guildID, s.State.User.ID, discordgo.PermissionViewAuditLogs) {
		return nil, nil, fmt.Errorf("missing audit log permissions in guild %s", guildID)
	}

	audit, err := s.GuildAuditLog(guildID, "", "", auditType, 3)
	if err != nil || len(audit.AuditLogEntries) == 0 {
		return nil, nil, fmt.Errorf("no recent audit entries")
	}

	var auditLog *discordgo.AuditLogEntry
	for _, entry := range audit.AuditLogEntries {
		if entry == nil || entry.UserID == s.State.User.ID {
			continue
		}

		entryTime, tsErr := discordgo.SnowflakeTimestamp(entry.ID)
		if tsErr != nil {
			continue
		}
		if time.Since(entryTime) > 5*time.Second {
			continue
		}

		auditLog = entry
		break
	}

	if auditLog == nil {
		return nil, nil, fmt.Errorf("no usable audit entry")
	}
	if GetGuildOwner(s, guildID) == auditLog.UserID {
		return auditLog, nil, fmt.Errorf("Whitelisted")
	}

	targetMember, err := s.State.Member(guildID, auditLog.UserID)
	if err != nil {
		targetMember, err = s.GuildMember(guildID, auditLog.UserID)
		if err != nil {
			return nil, nil, err
		}
	}

	if database.Database.IsWhitelisted(guildID, "users", auditLog.UserID, targetMember) {
		return auditLog, nil, fmt.Errorf("Whitelisted")
	}

	if len(auditLog.Changes) == 0 {
		return auditLog, []interface{}{}, nil
	}

	return auditLog, auditLog.Changes[0].NewValue, nil
}

func markAuditSeen(entryID string) bool {
	auditCacheMu.Lock()
	defer auditCacheMu.Unlock()

	now := time.Now()
	for id, ts := range auditSeen {
		if now.Sub(ts) > auditTTLWindow {
			delete(auditSeen, id)
		}
	}

	if _, ok := auditSeen[entryID]; ok {
		return false
	}
	auditSeen[entryID] = now
	return true
}

func addOffense(guildID, userID string, threshold int) bool {
	auditCacheMu.Lock()
	defer auditCacheMu.Unlock()

	now := time.Now()
	for key, record := range offenseCache {
		if now.Sub(record.Last) > offenseWindow {
			delete(offenseCache, key)
		}
	}

	key := guildID + ":" + userID
	record := offenseCache[key]
	if now.Sub(record.Last) > offenseWindow {
		record = offenseRecord{}
	}
	record.Last = now
	record.Count++
	offenseCache[key] = record

	if record.Count < threshold {
		return false
	}

	delete(offenseCache, key)
	return true
}

func FindInSlice(slice []string, item string) bool {
	for _, i := range slice {
		if i == item {
			return true
		}
	}
	return false
}

func GetGuildOwner(s *discordgo.Session, guildID string) string {
	guild, err := s.State.Guild(guildID)
	if err != nil {
		return ""
	}
	return guild.OwnerID
}

func HandleModeration(s *discordgo.Session, guildID, userID, reason string) error {
	data, err := database.Database.FindData(guildID)
	if err != nil {
		return err
	}

	moderationType, _ := data["moderation-type"].(string)
	if moderationType == "" {
		moderationType = "ban"
		_, _ = database.Database.SetData("$set", guildID, "moderation-type", moderationType)
	}

	finalReason := FormatReason(reason)
	if moderationType == "ban" {
		return s.GuildBanCreateWithReason(guildID, userID, finalReason, 0)
	}

	return s.GuildMemberDeleteWithReason(guildID, userID, finalReason)
}

func HandleModerationWithType(s *discordgo.Session, guildID, userID, reason, moderationType string) error {
	if moderationType == "" {
		moderationType = "ban"
	}

	finalReason := FormatReason(reason)
	if moderationType == "ban" {
		return s.GuildBanCreateWithReason(guildID, userID, finalReason, 0)
	}

	return s.GuildMemberDeleteWithReason(guildID, userID, finalReason)
}

func GetModerationType(guildID string) string {
	data, err := database.Database.FindData(guildID)
	if err != nil {
		return "ban"
	}

	moderationType, _ := data["moderation-type"].(string)
	if moderationType == "" {
		return "ban"
	}

	return moderationType
}

func HasPerms(s *discordgo.Session, m *discordgo.Message, guildID, userID string, permissions ...int) bool {
	if GetGuildOwner(s, guildID) == userID {
		return true
	}

	var (
		err   error
		guild *discordgo.Guild
		perms int64
	)

	switch m != nil {
	case true:
		perms, err = s.State.MessagePermissions(m)
		if err != nil {
			return false
		}
	case false:
		guild, err = s.State.Guild(guildID)
		if err != nil || len(guild.Channels) == 0 {
			return false
		}
		perms, err = s.State.UserChannelPermissions(userID, guild.Channels[0].ID)
		if err != nil {
			return false
		}
	}

	for _, perm := range permissions {
		if perms&(int64(perm)) == int64(perm) {
			return true
		}
	}

	return false
}

func HighestRole(s *discordgo.Session, guildID string, member *discordgo.Member) *discordgo.Role {
	guild, err := s.State.Guild(guildID)
	if err != nil {
		return nil
	}

	var highest *discordgo.Role
	for _, roleID := range member.Roles {
		for _, role := range guild.Roles {
			if roleID != role.ID {
				continue
			}
			if highest == nil || IsAbove(role, highest) {
				highest = role
			}
			break
		}
	}
	if highest == nil {
		defaultRole, _ := s.State.Role(guildID, guildID)
		return defaultRole
	}
	return highest
}

func IsAbove(r, r2 *discordgo.Role) bool {
	switch {
	case r.Position != r2.Position:
		return r.Position > r2.Position
	case r.ID == r2.ID:
		return false
	}
	return r.Position < r2.Position
}

func LogChannel(s *discordgo.Session, guildID, postData string) {
	data, err := database.Database.FindData(guildID)
	if err != nil {
		return
	}

	logChannel, _ := data["log-channel"].(string)
	if logChannel == "" || logChannel == "nil" {
		return
	}

	_, _ = s.ChannelMessageSend(logChannel, postData)
}

func SendAntiNukeLog(s *discordgo.Session, guildID string, info AntiNukeLogData) {
	data, err := database.Database.FindData(guildID)
	if err != nil {
		return
	}

	logChannel, _ := data["log-channel"].(string)
	SendAntiNukeLogToChannel(s, guildID, logChannel, info)
}

func SendAntiNukeLogToChannel(s *discordgo.Session, guildID, logChannel string, info AntiNukeLogData) {
	if logChannel == "" || logChannel == "nil" {
		return
	}

	if info.CounterAction == "" {
		info.CounterAction = GetModerationType(guildID)
	}

	criminal := "Unknown"
	if info.CriminalID != "" {
		criminal = fmt.Sprintf("<@%s>", info.CriminalID)
	}

	victim := "N/A"
	if info.VictimID != "" {
		victim = fmt.Sprintf("<@%s>", info.VictimID)
	}

	leftBlock := strings.Join([]string{
		fmt.Sprintf("👤 **Offender:** %s", criminal),
		fmt.Sprintf("🚨 **Violation:** %s", info.Crime),
		fmt.Sprintf("🎯 **Target:** %s", victim),
	}, "\n")

	rightBlock := strings.Join([]string{
		fmt.Sprintf("⚔️ **Action Taken:** %s", strings.ToUpper(info.CounterAction)),
		fmt.Sprintf("⏱️ **Response Time:** %s", info.ActionDuration),
		fmt.Sprintf("🆔 **Guild:** `%s`", guildID),
	}, "\n")

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🛡️ PlayZ Security Alert • %s", info.EventTitle),
		Description: "High-risk action detected and handled automatically.",
		Color:       AntiNukeLogColor,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "📌 Incident Snapshot", Value: leftBlock, Inline: true},
			{Name: "🧩 Enforcement Details", Value: rightBlock, Inline: true},
		},
		Footer:    &discordgo.MessageEmbedFooter{Text: "PlayZ Security • Auto Defense"},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	_, _ = s.ChannelMessageSendEmbed(logChannel, embed)
}

func MakeRequest(method, url, token string, body []byte) (resBody []byte, err error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	if token != "" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", token)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	resBody, err = io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	return resBody, nil
}

func handleAuditAction(s *discordgo.Session, guildID, reason string, auditType int, recovery RecoveryFunc) {
	started := time.Now()

	entry, _, err := FindAudit(s, guildID, auditType)
	if err != nil || entry == nil {
		return
	}
	if entry.UserID == s.State.User.ID {
		return
	}
	if GetGuildOwner(s, guildID) == entry.UserID {
		return
	}

	if !markAuditSeen(entry.ID) {
		return
	}

	guildData, err := database.Database.FindData(guildID)
	if err != nil {
		return
	}

	moderationType, _ := guildData["moderation-type"].(string)
	if moderationType == "" {
		moderationType = "ban"
	}
	logChannel, _ := guildData["log-channel"].(string)

	thresholdInt := 1
	if thresholdStr, ok := guildData["offense-threshold"].(string); ok {
		if parsed, parseErr := strconv.Atoi(thresholdStr); parseErr == nil && parsed > 0 {
			thresholdInt = parsed
		}
	}

	if !addOffense(guildID, entry.UserID, thresholdInt) {
		return
	}

	selfMember, err := s.State.Member(guildID, s.State.User.ID)
	if err != nil {
		selfMember, err = s.GuildMember(guildID, s.State.User.ID)
		if err != nil {
			return
		}
	}

	targetMember, err := s.State.Member(guildID, entry.UserID)
	if err != nil {
		targetMember, err = s.GuildMember(guildID, entry.UserID)
		if err != nil {
			return
		}
	}

	targetHighest := HighestRole(s, guildID, targetMember)
	selfHighest := HighestRole(s, guildID, selfMember)
	if targetHighest == nil || selfHighest == nil {
		return
	}

	if !IsAbove(selfHighest, targetHighest) || !HasPerms(s, nil, guildID, s.State.User.ID, discordgo.PermissionBanMembers) {
		return
	}

	if err := HandleModerationWithType(s, guildID, entry.UserID, reason, moderationType); err != nil {
		return
	}

	recoveryStatus := "not requested"
	if recovery != nil {
		if recoveryErr := recovery(entry); recoveryErr != nil {
			recoveryStatus = "failed"
		} else {
			recoveryStatus = "success"
		}
	}

	crimeText := reason
	if recovery != nil {
		crimeText = fmt.Sprintf("%s | recovery: %s", reason, recoveryStatus)
	}
	SendAntiNukeLogToChannel(s, guildID, logChannel, AntiNukeLogData{
		EventTitle:     reason,
		CriminalID:     entry.UserID,
		Crime:          crimeText,
		VictimID:       entry.TargetID,
		CounterAction:  moderationType,
		ActionDuration: FormatActionLatency(started, entry.ID),
	})
}

func ReadAudit(s *discordgo.Session, guildID, reason string, auditType int) {
	handleAuditAction(s, guildID, reason, auditType, nil)
}

func ReadAuditWithRecovery(s *discordgo.Session, guildID, reason string, auditType int, recovery RecoveryFunc) {
	handleAuditAction(s, guildID, reason, auditType, recovery)
}

func RemoveFromSlice(slice []string, item string) []string {
	returnItems := []string{}
	for _, i := range slice {
		if i == item {
			continue
		}
		returnItems = append(returnItems, i)
	}
	return returnItems
}
