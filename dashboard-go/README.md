## Project Structure

```
dashboard-go/
├── main.go              # Main Go application
├── go.mod               # Go dependencies
├── templates/
│   └── index.html       # HTML template
└── README.md            # Documentation
```

## Key Features:
- ✅ Pure Go stdlib - Uses only net/http and html/template
- ✅ Separated templates - HTML in templates/index.html
- ✅ Same functionality - Matches Python dashboard features
- ✅ JSON API - Available at /api/endpoints
- ✅ Lightweight - ~5-10 MB memory vs Python's ~20-40 MB
- ✅ Environment config - REDIS_ADDR, SERVER_PORT, etc.

## Setup:

```bash
cd dashboard-go
go mod download
go run main.go
```

Access at: `http://localhost:8080`

Go Template Syntax Differences (with Python Microdot framework):

Python/Jinja2: `{{ endpoint.field }}`
Go templates: `{{.Endpoint}}` or `{{$endpoint.Field}}`

The template includes a custom `add` function for index calculation. The Go dashboard runs on port 8080 (vs Python's 5000) so both can run simultaneously.
