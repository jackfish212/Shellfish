module github.com/jackfish212/grasp/cmd/grasp-server

go 1.24.3

require (
	github.com/jackfish212/grasp v0.0.0
	github.com/jackfish212/grasp/builtins v0.0.0
	github.com/jackfish212/grasp/mcpserver v0.0.0
)

require (
	github.com/rwtodd/Go.Sed v0.0.0-20250326002959-ba712dc84b62 // indirect
	github.com/thedevsaddam/gojsonq/v2 v2.5.2 // indirect
)

replace (
	github.com/jackfish212/grasp => ../../
	github.com/jackfish212/grasp/builtins => ../../builtins
	github.com/jackfish212/grasp/mcpserver => ../../mcpserver
)
