# null-receipts

stateless gRPC mircoservice that parses images of store receipts into JSON using Ollama or Gemini.

## usage

```bash
# if using ollama, at least qwen2.5vl:3b is recommended
ollama pull qwen2.5vl:3b

go run ./cmd/server
```

using a local model is preferred for privacy reasons, but `qwen2.5vl:3b` is quite power hungry, and it is the lightest model that works for the use-case. In testing, it can be run on my i5 7500/40GB RAM server without timing out. Anything lower spec will most likely fail due to ollama's internal timeout.

alternatively, you can use the generous Gemini API free tier that can process hundreads of images daily, if you are okay with sending your data to Google. An API can be obtained from the [Gemini AI studio](https://aistudio.google.com), along with the model name to use. `gemini-2.0-flash` works fine.


```bash

## configuration

all configuration is done via environment variables. copy `.env.example` to `.env` and adjust as needed.

| variable | default | description |
|----------|---------|-------------|
| `LISTEN_ADDRESS` | `127.0.0.1:55556` | server listen address (port, :port, or host:port) |
| `LOG_LEVEL` | `info` | log level: debug, info, warn, error |
| `LOG_FORMAT` | `text` | log format: text or json |
| `PROVIDER` | `ollama` | inference provider: ollama or gemini |
| `OLLAMA_HOST` | `http://127.0.0.1:11434` | ollama API endpoint |
| `OLLAMA_MODEL` | `qwen2.5vl:3b` | ollama model name |
| `GOOGLE_API_KEY` | | gemini API key (required when PROVIDER=gemini) |
| `GEMINI_MODEL` | `gemini-2.0-flash` | gemini model name |

this service implements routes defined in the [receipt_ocr.proto](https://github.com/xhos/null-protos/blob/main/null/v1/receipt_ocr.proto) file.

## development

```bash
# test cli for local model testing
go run cmd/test-cli/main.go image.jpg
```

to regenerate proto code:

```bash
regen
```

## 🌱 ecosystem

- [null-core](https://github.com/xhos/null-core) - main backend service
- [null-web](https://github.com/xhos/null-web) - frontend web application
- [null-mobile](https://github.com/xhos/null-mobile) - mobile appplication
- [null-protos](https://github.com/xhos/null-protos) - shared protobuf definitions
- [null-email-parser](https://github.com/xhos/null-email-parser) - email parsing service
- [null-statement-parser](https://github.com/xhos/null-statement-parser) - bank statement parsing cli tool
