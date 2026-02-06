module github.com/falcon-autotuning/instrument-server/runtime

go 1.25.2

require (
	github.com/falcon-autotuning/falcon-core-libs/go/falcon-core v0.0.1
	github.com/google/uuid v1.6.0
	github.com/mattn/go-sqlite3 v1.14.28
	github.com/nats-io/nats-server/v2 v2.11.4
	github.com/nats-io/nats.go v1.43.0
	github.com/spf13/cobra v1.9.1
	github.com/stretchr/testify v1.10.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/go-tpm v0.9.5 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/nats-io/jwt/v2 v2.7.4 // indirect
	github.com/nats-io/nkeys v0.4.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	golang.org/x/crypto v0.38.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/time v0.11.0 // indirect
)

replace github.com/falcon-autotuning/falcon-core-libs/go/falcon-core => ../../falcon-core-libs/go/falcon-core
