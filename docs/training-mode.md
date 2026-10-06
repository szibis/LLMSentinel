# Learning capture and future Jes serving

Learning mode keeps the primary commercial conversation direct: Claude Code talks to Anthropic and Codex talks to OpenAI. Client hooks send copies of completed turns to Sentinel's local collector. Sentinel does not proxy those commercial requests, receive provider credentials, or replace either client's provider settings.

Serving mode routes a client through Sentinel to the configured local model. The current lab uses its selected Qwen models; this does not establish that Jes matches a proprietary model. Switching a client to Jes serving is a later, explicit step after evaluation. Learning records are training candidates, not proof of correctness, commercial parity, or automatic permission to train.

## Start an independent collector

The gateway supports `--mode learning`, `--mode serving` (the default), and explicitly authorized `--mode hybrid`. See [hybrid routing and billing](hybrid-billing.md) for commercial/local mixing with capture. Learning mode requires `--training-dir`, enables capture, and refuses inference/proxy requests. Start its collector separately from an already active local lab, for example:

```sh
./bin/sentinel-gateway --listen 127.0.0.1:19094 \
  --mode learning --training-dir /private/tmp/sentinel-learning
```

Build the executable with `make gateway-build` first. This command does not restart the serving gateway on port 19090 or either MLX runtime. Configure the opt-in client hooks to send copied records to `http://127.0.0.1:19094/sentinel/training/events`. They must leave the primary provider connection direct. Do not put provider keys in hook records.

For optional local evaluation capture while serving, use `--mode serving --training-mode --training-dir /private/tmp/sentinel-local-evaluation`. Capture is disabled by default in serving mode. `--training-max-bytes` defaults to 33554432 bytes per file.

There are currently no automatic commercial API requests, local shadow replays, model updates, or training jobs in the recorder. Commercial hook ingestion records what the client reports; it does not independently verify provider identity, usage accounting or answer quality. A future comparison runner can pair reference turns with out-of-band local replays without putting Sentinel in the commercial primary path.

## Private, bounded records

Use a dedicated capture directory outside the Git checkout. The recorder creates it with mode `0700` and JSONL files with mode `0600`; existing public directories/files and symlink leaves are refused. The directory and its parents should be under the user's control. Files are `training.jsonl` and one rotated predecessor, `training.jsonl.1`. Each file stays within the configured byte limit; the default retains at most 64 MiB across both files. An individual event larger than the limit is dropped. Reducing the configured limit below the size of an existing capture file requires moving that file aside first.

Records include timestamp, client, provider, request/attempt identifiers, optional pairing/reference metadata, model and role, thinking/budget configuration, input conversation, output, reported usage, latency, protocol acceptance and quality metadata. Local inference attempts can include invalid output and correction attempts; correlate them by request ID rather than treating them as separate successful turns. Missing usage is unknown, not measured zero. `accepted` describes the relevant protocol/capture result and does not establish semantic correctness.

Opting in authorizes conversation copies in this private directory, including tool evidence and code present in the captured payload. The recorder redacts common credential keys, bearer tokens, `sk-` keys, AWS access-key identifiers and private-key blocks. Redaction is best effort: unusual credential formats, personal information, private code and ordinary conversation text can remain. Treat the capture files as sensitive and inspect/redact them before sharing or exporting a dataset.

Capture errors emit fixed diagnostic messages without conversation content, paths, credentials or raw storage errors. A recorder write failure does not fail the model request. Rotation is serialized within the gateway; use one gateway process per capture directory.

## Quality gates before dataset admission

Every captured event defaults to `quality.status = "unscored"` and always has `quality.training_eligible = false`. A successfully parsed local tool call may report protocol validity, while hook evidence remains untrusted capture. Neither is sufficient for admission to a training set. Invalid or truncated tool calls, unsupported schemas, rejected tool-choice behavior and failed generations should remain identifiable as failures.

Before promoting candidates, evaluate task success against actual tool-result evidence, argument/schema validity, unsupported claims, refusal behavior, deterministic regression tasks and reviewed reference comparisons. Split datasets by conversation/task so neighboring turns do not leak across training and evaluation. Keep provenance, pairing identifiers and explicit reviewer decisions in the curated dataset. Passing these gates may justify an experiment; it does not by itself demonstrate proprietary-model parity.
