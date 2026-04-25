// Package classify identifies task types from prompt text for routing decisions.
package classify

import (
	"regexp"
	"strings"
)

// TaskType represents a classified task domain.
type TaskType string

const (
	TaskConcurrency  TaskType = "concurrency"
	TaskParsing      TaskType = "parsing"
	TaskOptimization TaskType = "optimization"
	TaskDebugging    TaskType = "debugging"
	TaskArchitecture TaskType = "architecture"
	TaskSecurity     TaskType = "security"
	TaskDatabase     TaskType = "database"
	TaskNetworking   TaskType = "networking"
	TaskTesting      TaskType = "testing"
	TaskDevOps       TaskType = "devops"
	TaskGeneral      TaskType = "general"
)

// AllTaskTypes returns all non-general task types.
func AllTaskTypes() []TaskType {
	return []TaskType{
		TaskConcurrency, TaskParsing, TaskOptimization, TaskDebugging,
		TaskArchitecture, TaskSecurity, TaskDatabase, TaskNetworking,
		TaskTesting, TaskDevOps,
	}
}

type rule struct {
	taskType TaskType
	pattern  *regexp.Regexp
}

var rules = []rule{
	{TaskConcurrency, regexp.MustCompile(`(?i)\b(race|concurrent|thread|deadlock|atomic|mutex|lock|semaphore|goroutine|channel|async|await|parallel)\b`)},
	{TaskParsing, regexp.MustCompile(`(?i)\b(regex|parse|grammar|tokenize|lexer|ast|syntax|state.machine)\b`)},
	{TaskOptimization, regexp.MustCompile(`(?i)\b(optimi[zs]\w*|perform\w*|speed|latency|throughput|benchmark\w*|profil\w*|cache|memory.leak)\b`)},
	{TaskDebugging, regexp.MustCompile(`(?i)\b(debug|traceback|segfault|panic|stack.trace|core.dump|breakpoint)\b`)},
	{TaskArchitecture, regexp.MustCompile(`(?i)\b(architec|design|structur|microservice|monolith|event.driven|system.design|domain.driven)\b`)},
	{TaskSecurity, regexp.MustCompile(`(?i)\b(crypto|security|encrypt|auth|tls|ssl|oauth|jwt|certificate|xss|sqli|csrf)\b`)},
	{TaskDatabase, regexp.MustCompile(`(?i)\b(database|sql|query|migration|schema|index|transaction|postgres|mysql|redis)\b`)},
	{TaskNetworking, regexp.MustCompile(`(?i)\b(network|socket|tcp|udp|http|dns|proxy|websocket|grpc|load.balanc)\b`)},
	{TaskTesting, regexp.MustCompile(`(?i)\b(test|spec|assert|mock|stub|fixture|coverage|tdd|bdd)\b`)},
	{TaskDevOps, regexp.MustCompile(`(?i)\b(deploy|docker|ci|cd|pipeline|kubernetes|helm|terraform|ansible|jenkins)\b`)},
}

// Classify determines the task type from a prompt.
// Returns the first matching domain type, or TaskGeneral if none match.
func Classify(prompt string) TaskType {
	lower := strings.ToLower(prompt)
	for _, r := range rules {
		if r.pattern.MatchString(lower) {
			return r.taskType
		}
	}
	return TaskGeneral
}
