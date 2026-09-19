module github.com/knowoff/knowoff/tools/mediapack

go 1.25.0

replace github.com/knowoff/knowoff/server => ../../server

require (
	github.com/knowoff/knowoff/server v0.0.0-00010101000000-000000000000
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/google/uuid v1.6.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)
