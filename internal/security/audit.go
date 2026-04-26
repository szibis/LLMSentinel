package security

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditLogger logs security events
type AuditLogger struct {
	logDir       string
	currentFile  *os.File
	mu           sync.Mutex
	eventCounter int64
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(logDir string) (*AuditLogger, error) {
	// Create log directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return nil, err
	}

	logger := &AuditLogger{
		logDir: logDir,
	}

	// Create initial log file
	if err := logger.rotateLogFile(); err != nil {
		return nil, err
	}

	return logger, nil
}

// LogSecurityEvent logs a security event
func (al *AuditLogger) LogSecurityEvent(eventType, severity string, details map[string]interface{}) error {
	al.mu.Lock()
	defer al.mu.Unlock()

	timestamp := time.Now().Format(time.RFC3339)
	al.eventCounter++

	// Build log entry
	logEntry := fmt.Sprintf("[%s] [%s] [%s] #%d", timestamp, eventType, severity, al.eventCounter)

	// Add details
	if details != nil {
		for key, value := range details {
			logEntry += fmt.Sprintf(" %s=%v", key, value)
		}
	}

	logEntry += "\n"

	// Write to log file
	if _, err := al.currentFile.WriteString(logEntry); err != nil {
		return err
	}

	// Also write critical events to stderr
	if severity == "CRITICAL" {
		fmt.Fprintf(os.Stderr, "SECURITY ALERT: %s\n", logEntry)
	}

	return nil
}

// LogInjectionAttempt logs an injection attack attempt
func (al *AuditLogger) LogInjectionAttempt(ip, patternType, input string) error {
	details := map[string]interface{}{
		"ip":            ip,
		"pattern_type":  patternType,
		"input_sample":  truncateString(input, 100),
		"timestamp":     time.Now().Unix(),
	}

	return al.LogSecurityEvent("INJECTION_ATTEMPT", "HIGH", details)
}

// LogRateLimitTriggered logs a rate limit trigger
func (al *AuditLogger) LogRateLimitTriggered(ip string) error {
	details := map[string]interface{}{
		"ip":        ip,
		"timestamp": time.Now().Unix(),
	}

	return al.LogSecurityEvent("RATE_LIMIT_TRIGGERED", "MEDIUM", details)
}

// LogValidationFailure logs a validation failure
func (al *AuditLogger) LogValidationFailure(ip, reason string) error {
	details := map[string]interface{}{
		"ip":     ip,
		"reason": reason,
	}

	return al.LogSecurityEvent("VALIDATION_FAILURE", "MEDIUM", details)
}

// LogUnauthorizedAccess logs an unauthorized access attempt
func (al *AuditLogger) LogUnauthorizedAccess(ip, resource string) error {
	details := map[string]interface{}{
		"ip":       ip,
		"resource": resource,
	}

	return al.LogSecurityEvent("UNAUTHORIZED_ACCESS", "HIGH", details)
}

// rotateLogFile creates a new log file
func (al *AuditLogger) rotateLogFile() error {
	if al.currentFile != nil {
		al.currentFile.Close()
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	logPath := filepath.Join(al.logDir, fmt.Sprintf("security_%s.log", timestamp))

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}

	al.currentFile = file
	return nil
}

// Close closes the audit logger
func (al *AuditLogger) Close() error {
	al.mu.Lock()
	defer al.mu.Unlock()

	if al.currentFile != nil {
		return al.currentFile.Close()
	}

	return nil
}

// GetEventCount returns the total number of events logged
func (al *AuditLogger) GetEventCount() int64 {
	al.mu.Lock()
	defer al.mu.Unlock()

	return al.eventCounter
}

// SecurityEvent represents a security event
type SecurityEvent struct {
	Timestamp   time.Time
	Type        string
	Severity    string
	IP          string
	PatternType string
	Details     map[string]interface{}
}

func truncateString(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
