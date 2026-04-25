package classify

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		prompt   string
		expected TaskType
	}{
		{"Fix the race condition in concurrent code", TaskConcurrency},
		{"How do I use mutex locks with goroutines?", TaskConcurrency},
		{"Parse the JSON response from the API", TaskParsing},
		{"Write a regex to match email addresses", TaskParsing},
		{"Optimize the database query performance", TaskOptimization},
		{"Profile the memory usage of this function", TaskOptimization},
		{"Debug the segfault in the C extension", TaskDebugging},
		{"Design a microservice architecture", TaskArchitecture},
		{"Implement OAuth2 JWT authentication", TaskSecurity},
		{"Write a SQL migration for the users table", TaskDatabase},
		{"Configure the TCP socket server", TaskNetworking},
		{"Write unit tests with mock dependencies", TaskTesting},
		{"Set up a Kubernetes deployment pipeline", TaskDevOps},
		{"What is 2+2?", TaskGeneral},
		{"Hello world", TaskGeneral},
		{"Read the README file", TaskGeneral},
	}

	for _, tt := range tests {
		t.Run(tt.prompt, func(t *testing.T) {
			got := Classify(tt.prompt)
			if got != tt.expected {
				t.Errorf("Classify(%q) = %q, want %q", tt.prompt, got, tt.expected)
			}
		})
	}
}
