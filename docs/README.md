# epubaudio API Documentation 📖

This folder contains the official OpenAPI 3.1 specification for the `epubaudio` REST and Server-Sent Events (SSE) API.

* **Spec File**: [`openapi.yaml`](./openapi.yaml)

---

## 🛠️ Viewing the Documentation

### Option 1: Swagger Editor (Browser)
1. Open [editor.swagger.io](https://editor.swagger.io/).
2. Select **File > Import File** and upload [`docs/openapi.yaml`](./openapi.yaml).
3. Browse interactive documentation, schemas, and example payloads.

### Option 2: VS Code / Cursor Extension
Install the **OpenAPI (Swagger) Editor** or **Redocly** extension to view rendered interactive API documentation directly within your editor.

### Option 3: Local CLI Viewer (Redoc)
You can render a documentation page with zero installation:
```bash
npx @redocly/cli preview-docs docs/openapi.yaml
```

---

## 📡 API Overview

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/api/upload` | Upload `.epub` file and extract book metadata |
| `GET` | `/api/voices` | List TTS voices with optional language filter |
| `POST` | `/api/convert` | Start background audiobook synthesis task |
| `GET` | `/api/progress` | Stream real-time chapter & chunk progress (SSE) |
| `GET` | `/api/download` | Download generated `.m4b` or `.mp3` audio file |

