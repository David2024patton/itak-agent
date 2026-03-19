module github.com/David2024patton/iTaKAgent

go 1.26.0

require (
	github.com/David2024patton/iTaKDatabase v0.0.0-00010101000000-000000000000
	github.com/gogpu/gputypes v0.2.0
	github.com/gogpu/wgpu v0.19.6
	github.com/google/cel-go v0.27.0
	github.com/google/go-github/v69 v69.2.0
	github.com/gorilla/websocket v1.5.3
	gopkg.in/yaml.v3 v3.0.1
)

require (
	cel.dev/expr v0.25.1 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/go-webgpu/goffi v0.4.2 // indirect
	github.com/gogpu/naga v0.14.5 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	go.etcd.io/bbolt v1.4.3 // indirect
	golang.org/x/exp v0.0.0-20240823005443-9b4947da3948 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240826202546-f6391c0de4c7 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240826202546-f6391c0de4c7 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
)

replace github.com/David2024patton/iTaKDatabase => ../Database
