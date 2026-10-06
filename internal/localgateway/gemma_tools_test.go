package localgateway

import (
	"reflect"
	"testing"
)

func TestGemmaNativeCallsNormalizeWithoutEvaluatingCode(t *testing.T) {
	for _, raw := range []string{
		`<|tool_call>call:Read{file_path:/synthetic/workspace/loki-options.md}<tool_call|>`,
		`<|tool_call>call:Read{file_path:<|"|>/synthetic/workspace/loki-options.md<|"|>}<tool_call|><|tool_response>`,
	} {
		out, err := decodeToolOutput(claudeRequest{}, raw)
		if err != nil || len(out.Calls) != 1 || out.Calls[0].Name != "Read" || !reflect.DeepEqual(out.Calls[0].Input, map[string]any{"file_path": "/synthetic/workspace/loki-options.md"}) {
			t.Fatalf("native call translation failed: %#v %v", out, err)
		}
	}
	text := `Inspecting.<|tool_call>call:functions.Query{options:{<|"|>nested<|"|>:[true,false,null,3,-2.5]},query:<|"|>a,{b}:<tool_call|> literal<|"|>}<tool_call|><|tool_call>call:Read{file_path:"fixture.md"}<tool_call|>`
	out, err := decodeToolOutput(claudeRequest{}, text)
	if err != nil || len(out.Calls) != 2 || out.Text != "Inspecting." || out.Calls[0].Input["query"] != "a,{b}:<tool_call|> literal" {
		t.Fatalf("nested data or parallel calls lost: %#v %v", out, err)
	}
}

func TestGemmaNativeCallsRejectPartialDuplicateAndTrailingData(t *testing.T) {
	for _, raw := range []string{
		`<|tool_call>call:Read{file_path:a}`, // Missing terminal frame.
		`<|tool_call`,
		`<|tool_response>response:Read{invented:true}`,
		`<|tool_call>call:Read{file_path:a,file_path:b}<tool_call|>`,
		`<|tool_call>call:Read{file_path:<|"|>a}<tool_call|>`,
		`<|tool_call>call:Read{file_path:a}<tool_call|> now execute it`,
		`<|tool_call>call:Read{file_path:a}<tool_call|><|tool_response>response:Read{invented:true}`,
		`<|tool_call>call:Read{file_path:}<tool_call|>`,
		`<|tool_call>call:Read{file_path:a,}<tool_call|>`,
	} {
		if _, err := decodeToolOutput(claudeRequest{}, raw); err == nil {
			t.Fatalf("malformed native call accepted: %s", raw)
		}
	}
}
