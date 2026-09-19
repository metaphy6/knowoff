module github.com/knowoff/knowoff/server/tools/gamebot

go 1.25.0

require github.com/gorilla/websocket v1.5.3

replace github.com/knowoff/knowoff/server => ../../server

require (
	github.com/google/uuid v1.6.0
	github.com/knowoff/knowoff/server v0.0.0-00010101000000-000000000000
	github.com/lib/pq v1.12.3
	gopkg.in/yaml.v3 v3.0.1
)

require (
	cloud.google.com/go/compute/metadata v0.8.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/chai2010/webp v1.4.0 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/golang-migrate/migrate/v4 v4.19.1 // indirect
	github.com/redis/go-redis/v9 v9.22.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)
