package hook

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestStdioProtocol(t *testing.T) {
	oldIn, oldOut := os.Stdin, os.Stdout
	t.Cleanup(func() { os.Stdin, os.Stdout = oldIn, oldOut })
	for _, tc := range []struct {
		body string
		fail bool
	}{{`{"prompt":"hello 🌍"}`, false}, {`{`, true}} {
		f, err := os.CreateTemp(t.TempDir(), "input")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.WriteString(tc.body); err != nil {
			t.Fatal(err)
		}
		if _, err = f.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		os.Stdin = f
		in, err := ReadInput()
		if (err != nil) != tc.fail {
			t.Fatalf("ReadInput: %v", err)
		}
		if !tc.fail && in.Prompt != "hello 🌍" {
			t.Fatalf("input: %+v", in)
		}
		f.Close()
	}
	closed, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	os.Stdin = closed
	if _, err := ReadInput(); err == nil || !strings.Contains(err.Error(), "reading stdin") {
		t.Fatalf("closed input: %v", err)
	}
	for _, out := range []*Output{PassThrough(), WithHint("try opus")} {
		f, err := os.CreateTemp(t.TempDir(), "output")
		if err != nil {
			t.Fatal(err)
		}
		os.Stdout = f
		if err := WriteOutput(out); err != nil {
			t.Fatal(err)
		}
		f.Seek(0, 0)
		var decoded Output
		if err := json.NewDecoder(f).Decode(&decoded); err != nil {
			t.Fatal(err)
		}
		if !decoded.Continue || !decoded.SuppressOutput {
			t.Fatalf("output: %+v", decoded)
		}
		if out.HookOutput != nil && decoded.HookOutput["additionalContext"] != "try opus" {
			t.Fatal(decoded)
		}
		f.Close()
	}
	os.Stdout = closed
	if err := WriteOutput(PassThrough()); err == nil {
		t.Fatal("closed output accepted")
	}
	if err := WriteOutput(&Output{HookOutput: map[string]interface{}{"bad": make(chan int)}}); err == nil {
		t.Fatal("unsupported JSON accepted")
	}
}
