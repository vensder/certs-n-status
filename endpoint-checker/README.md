# Endpoint checker in Go with Redis storage.

**Key Features:**
1. **HTTP Status Checks** - Every 1 minute (configurable)
2. **SSL Certificate Expiration** - Every 1 hour (configurable)
3. **Redis Storage** with the key structure you suggested:
   - `status:<url>` → HTTP status code
   - `ssl:<url>` → SSL expiration as Unix timestamp
   - `status_updated:<url>` → Last status check timestamp
   - `ssl_updated:<url>` → Last SSL check timestamp

4. **Concurrent checking** using goroutines for better performance
5. **Environment variable configuration** for flexibility
6. **Error handling** and logging

**To use this:**

1. **Install dependencies:**

```bash
go mod init endpoint-checker
go get github.com/redis/go-redis/v9
```

2. **Create `endpoints.lst`:**

```
https://google.com
https://github.com
example.com
# This is a comment
http://httpbin.org/status/200
```

3. **Run Redis:**

```bash
docker run -d -p 6379:6379 redis:alpine
```

4. **Run the checker:**

```bash
go run main.go
```

**Configuration via environment variables:**

```bash
STATUS_CHECK_INTERVAL=30s SSL_CHECK_INTERVAL=2h ENDPOINTS_FILE=mylist.txt go run main.go
```

**Redis data structure benefits:**
- Fast lookups by URL
- Unix timestamps are efficient (int64)
- Easy to calculate "days left" in any language
- Separate update timestamps let you know data freshness

