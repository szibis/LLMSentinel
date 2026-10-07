package qwensmoke

import (
	"testing"
)

func accountingSnapshot(requests, input, output float64, last document) document {
	return document{"stats": document{"requests": requests, "tokens_generated": output, "last_generation": last}, "prompt_cache": document{"processed_tokens": input, "reused_tokens": float64(0)}}
}

func TestNativeAccountingIncludesEveryRecoveryAttempt(t *testing.T) {
	for _, attempts := range []float64{1, 2} {
		for _, fault := range []string{"", "input", "output", "requests", "missing", "fractional", "reset", "prompt-reset", "overflow", "not-native", "truncated"} {
			t.Run(fault+"/"+map[float64]string{1: "one", 2: "two"}[attempts], func(t *testing.T) {
				before := accountingSnapshot(10, 100, 50, nil)
				last := document{"native_generation_metadata": true, "finish_reason": "stop", "prompt_tokens": float64(20), "generation_tokens": float64(4)}
				after := accountingSnapshot(10+attempts, 100+20*attempts, 50+4*attempts, last)
				usage := document{"input_tokens": 20 * attempts, "output_tokens": 4 * attempts}
				switch fault {
				case "input":
					usage["input_tokens"] = float64(19)
				case "output":
					usage["output_tokens"] = float64(3)
				case "requests":
					mapping(after["stats"])["requests"] = float64(13)
				case "missing":
					delete(mapping(after["prompt_cache"]), "processed_tokens")
				case "fractional":
					mapping(after["stats"])["requests"] = 10.5
				case "reset":
					mapping(after["stats"])["tokens_generated"] = float64(1)
				case "prompt-reset":
					mapping(before["prompt_cache"])["reused_tokens"] = float64(50)
					mapping(after["prompt_cache"])["reused_tokens"] = float64(40)
					usage["input_tokens"] = 20*attempts - 10
				case "overflow":
					mapping(after["prompt_cache"])["processed_tokens"] = float64(1<<53 - 1)
					mapping(after["prompt_cache"])["reused_tokens"] = float64(2)
				case "not-native":
					last["native_generation_metadata"] = false
				case "truncated":
					last["finish_reason"] = "length"
				}
				err := validateNativeAccountingWindow(before, after, usage)
				if (err == nil) != (fault == "") {
					t.Fatalf("attempts=%v fault=%s err=%v", attempts, fault, err)
				}
			})
		}
	}
}
