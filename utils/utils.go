package utils

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
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

type AuditActionOptions struct {
	GuildData database.GuildData
}

type offenseRecord struct {
	Count int
	Last  time.Time
}

var (
	auditCacheMu          sync.Mutex
	auditSeen             = make(map[string]time.Time)
	offenseCache          = make(map[string]offenseRecord)
	offenseWindow         = 12 * time.Second
	auditTTLWindow        = 2 * time.Minute
	nextAuditCacheSweep   time.Time
	nextOffenseCacheSweep time.Time
)

var antiNukeTraceEnabled = func() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("PLAYZ_TRACE_ANTINUKE")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}()

func FormatReason(reason string) string {
	return fmt.Sprintf("%s | %s", ReasonPrefix, reason)
}

func FormatActionLatency(started time.Time, auditEntryID string) string {
	processSeconds := time.Since(started).Seconds()
	if processSeconds < 0 {
		processSeconds = 0
	}

	return fmt.Sprintf("%.2fs", processSeconds)
}

func traceAntiNuke(guildID, event, step string, started time.Time) {
	if !antiNukeTraceEnabled {
		return
	}

	fmt.Printf("[AntiNuke Trace] guild=%s event=%s step=%s elapsed=%s\n", guildID, event, step, time.Since(started).Round(time.Millisecond))
}

func getMember(s *discordgo.Session, guildID, userID string) (*discordgo.Member, error) {
	member, err := s.State.Member(guildID, userID)
	if err == nil {
		return member, nil
	}

	return s.GuildMember(guildID, userID)
}

func getThreshold(data database.GuildData) int {
	threshold := 1
	if thresholdStr, ok := data["offense-threshold"].(string); ok {
		if parsed, err := strconv.Atoi(thresholdStr); err == nil && parsed > 0 {
			threshold = parsed
		}
	}
	return threshold
}

func FindAudit(s *discordgo.Session, guildID string, auditType int) (*discordgo.AuditLogEntry, interface{}, error) {
	return findAuditWithOptions(s, guildID, auditType, "")
}

func findAuditWithOptions(s *discordgo.Session, guildID string, auditType int, ownerID string) (*discordgo.AuditLogEntry, interface{}, error) {
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

	if ownerID == "" {
		ownerID = GetGuildOwner(s, guildID)
	}
	if ownerID == auditLog.UserID {
		return auditLog, nil, fmt.Errorf("Whitelisted")
	}

	targetMember, err := getMember(s, guildID, auditLog.UserID)
	if err != nil {
		return nil, nil, err
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
	if nextAuditCacheSweep.IsZero() || now.After(nextAuditCacheSweep) {
		for id, ts := range auditSeen {
			if now.Sub(ts) > auditTTLWindow {
				delete(auditSeen, id)
			}
		}
		nextAuditCacheSweep = now.Add(auditTTLWindow / 2)
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
	if nextOffenseCacheSweep.IsZero() || now.After(nextOffenseCacheSweep) {
		for key, record := range offenseCache {
			if now.Sub(record.Last) > offenseWindow {
				delete(offenseCache, key)
			}
		}
		nextOffenseCacheSweep = now.Add(offenseWindow / 2)
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

	roleByID := make(map[string]*discordgo.Role, len(guild.Roles))
	for _, role := range guild.Roles {
		roleByID[role.ID] = role
	}

	var highest *discordgo.Role
	for _, roleID := range member.Roles {
		role := roleByID[roleID]
		if role == nil {
			continue
		}
		if highest == nil || IsAbove(role, highest) {
			highest = role
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

func moderationPermission(moderationType string) int {
	if strings.EqualFold(moderationType, "kick") {
		return discordgo.PermissionKickMembers
	}
	return discordgo.PermissionBanMembers
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

func handleAuditAction(s *discordgo.Session, guildID, reason string, auditType int, recovery RecoveryFunc, options AuditActionOptions) {
	started := time.Now()
	traceAntiNuke(guildID, reason, "start", started)

	guildData := options.GuildData
	var err error
	if guildData == nil {
		guildData, err = database.Database.FindData(guildID)
		if err != nil {
			traceAntiNuke(guildID, reason, "guild-data-miss", started)
			return
		}
	}

	ownerID := GetGuildOwner(s, guildID)
	entry, _, err := findAuditWithOptions(s, guildID, auditType, ownerID)
	if err != nil || entry == nil {
		traceAntiNuke(guildID, reason, "audit-miss", started)
		return
	}
	traceAntiNuke(guildID, reason, "audit-found", started)

	if entry.UserID == s.State.User.ID {
		traceAntiNuke(guildID, reason, "self-entry-skip", started)
		return
	}
	if ownerID == entry.UserID {
		traceAntiNuke(guildID, reason, "owner-skip", started)
		return
	}

	if !markAuditSeen(entry.ID) {
		traceAntiNuke(guildID, reason, "duplicate-entry-skip", started)
		return
	}

	moderationType, _ := guildData["moderation-type"].(string)
	if moderationType == "" {
		moderationType = "ban"
	}
	logChannel, _ := guildData["log-channel"].(string)
	thresholdInt := getThreshold(guildData)

	if !addOffense(guildID, entry.UserID, thresholdInt) {
		traceAntiNuke(guildID, reason, "threshold-not-met", started)
		return
	}

	type memberResult struct {
		member *discordgo.Member
		err    error
	}
	selfResultCh := make(chan memberResult, 1)
	targetResultCh := make(chan memberResult, 1)

	go func() {
		member, memberErr := getMember(s, guildID, s.State.User.ID)
		selfResultCh <- memberResult{member: member, err: memberErr}
	}()
	go func() {
		member, memberErr := getMember(s, guildID, entry.UserID)
		targetResultCh <- memberResult{member: member, err: memberErr}
	}()

	selfResult := <-selfResultCh
	if selfResult.err != nil {
		traceAntiNuke(guildID, reason, "self-member-miss", started)
		return
	}

	targetResult := <-targetResultCh
	if targetResult.err != nil {
		traceAntiNuke(guildID, reason, "target-member-miss", started)
		return
	}

	selfMember := selfResult.member
	targetMember := targetResult.member

	targetHighest := HighestRole(s, guildID, targetMember)
	selfHighest := HighestRole(s, guildID, selfMember)
	if targetHighest == nil || selfHighest == nil {
		traceAntiNuke(guildID, reason, "role-check-miss", started)
		return
	}

	if !IsAbove(selfHighest, targetHighest) || !HasPerms(s, nil, guildID, s.State.User.ID, moderationPermission(moderationType)) {
		traceAntiNuke(guildID, reason, "hierarchy-skip", started)
		return
	}

	if err := HandleModerationWithType(s, guildID, entry.UserID, reason, moderationType); err != nil {
		traceAntiNuke(guildID, reason, "moderation-failed", started)
		return
	}
	traceAntiNuke(guildID, reason, "moderation-complete", started)

	actionDuration := FormatActionLatency(started, entry.ID)
	go func() {
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
			ActionDuration: actionDuration,
		})
		traceAntiNuke(guildID, reason, "log-sent", started)
	}()
}

func ReadAudit(s *discordgo.Session, guildID, reason string, auditType int) {
	handleAuditAction(s, guildID, reason, auditType, nil, AuditActionOptions{})
}

func ReadAuditWithRecovery(s *discordgo.Session, guildID, reason string, auditType int, recovery RecoveryFunc) {
	handleAuditAction(s, guildID, reason, auditType, recovery, AuditActionOptions{})
}

func ReadAuditWithData(s *discordgo.Session, guildID, reason string, auditType int, guildData database.GuildData) {
	handleAuditAction(s, guildID, reason, auditType, nil, AuditActionOptions{GuildData: guildData})
}

func ReadAuditWithRecoveryData(s *discordgo.Session, guildID, reason string, auditType int, guildData database.GuildData, recovery RecoveryFunc) {
	handleAuditAction(s, guildID, reason, auditType, recovery, AuditActionOptions{GuildData: guildData})
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
