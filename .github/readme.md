# null-receipts

stateless gRPC mircoservice that parses images of store receipts into JSON using Ollama or Gemini.

## usage

```bash
# if using ollama, qwen2.5vl:3b is recommended
ollama pull qwen2.5vl:3b

go run ./cmd/server
```

## configuration

| flag    | default | description          |
|---------|---------|----------------------|
| `-port` | 55556   | gRPC listen port     |
| `-json` | false   | JSON structured logs |

| env            | default                  | description    |
|----------------|--------------------------|----------------|
| `OLLAMA_HOST`  | `http://127.0.0.1:11434` | Ollama API URL |
| `OLLAMA_MODEL` | `qwen2.5vl:3b`           | Model name     |

## proto

```protobuf
service ReceiptOCRService {
  rpc ParseReceipt(ParseReceiptRequest) returns (ParseReceiptResponse);
  rpc Health(HealthRequest) returns (HealthResponse);
}
```

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
