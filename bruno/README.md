# epubaudio Bruno API Collection 🐶

This folder contains a complete [Bruno](https://www.usebruno.com/) collection testing the `epubaudio` REST and SSE API.

---

## 🚀 How to Run the Tests

### Option A: Using the Bruno Desktop App (GUI)

1. Open **Bruno**.
2. Click **Open Collection** and select this `bruno/` directory.
3. In the top-right corner, select the **Local** environment (`baseUrl: http://localhost:8080`).
4. Make sure your server is running:
   ```bash
   make web
   # or: ./epubaudio -web
   ```
5. Click **Run** on the collection to execute all tests sequentially!

---

### Option B: Using the Bruno CLI (`bru`)

You can run the entire test suite directly from your terminal:

```bash
# Using npx (no global install required):
npx @usebruno/cli run --env Local

# Or if you have bru installed globally (npm i -g @usebruno/cli):
bru run --env Local
```

---

## 📋 Test Requests Included

| Seq | Request | Method | Endpoint | Description |
|---|---|---|---|---|
| `01` | **Web UI SPA (Root)** | `GET` | `/` | Verifies static file serving and SPA root element |
| `02` | **List Voices (Edge-TTS)** | `GET` | `/api/voices?synth=edge&lang=es` | Tests voice listing, JSON tags, and schema validation |
| `03` | **List Voices (macOS Say)** | `GET` | `/api/voices?synth=macos&lang=en` | Tests offline native engine voice enumeration |
| `04` | **Upload EPUB File** | `POST` | `/api/upload` | Uploads multipart `.epub` and asserts extracted book metadata |
| `05` | **Start Conversion Job** | `POST` | `/api/convert` | Starts chapter 0 conversion task and extracts `jobId` |
| `06` | **Get Progress (SSE)** | `GET` | `/api/progress?jobId={{jobId}}` | Verifies Server-Sent Events stream headers and connection |
| `07` | **Download Generated Audio** | `GET` | `/api/download?file=...` | Verifies file serving with `Content-Disposition: attachment` |
| `08` | **Error - Invalid Upload** | `POST` | `/api/upload` | Asserts `400 Bad Request` when uploading non-epub files |
| `09` | **Error - Nonexistent Download** | `GET` | `/api/download` | Asserts `404 Not Found` for missing audio files |
| `10` | **Error - Convert Missing Book** | `POST` | `/api/convert` | Asserts `404 Not Found` when converting unuploaded books |

