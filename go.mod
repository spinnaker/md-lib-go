module github.com/spinnaker/md-lib-go

go 1.13

require (
	github.com/AlecAivazis/survey/v2 v2.0.7
	github.com/fatih/color v1.9.0 // indirect
	github.com/golang/protobuf v1.3.5 // indirect
	github.com/hashicorp/go-multierror v1.1.0 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/mattn/go-colorable v0.1.6 // indirect
	github.com/mgutz/ansi v0.0.0-20170206155736-9520e82c474b
	github.com/mitchellh/cli v1.1.0 // indirect
	github.com/palantir/stacktrace v0.0.0-20161112013806-78658fd2d177
	github.com/posener/complete v1.2.3 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/spinnaker/spin v0.4.0
	github.com/stretchr/testify v1.5.0
	github.com/xlab/treeprint v1.0.0
	golang.org/x/crypto v0.0.0-20200214034016-1d94cc7ab1c6
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e // indirect
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d // indirect
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/sys v0.0.0-20200331124033-c3d80250170d // indirect
	google.golang.org/appengine v1.6.5 // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
	gopkg.in/yaml.v2 v2.2.8
	gopkg.in/yaml.v3 v3.0.0-20200121175148-a6ecf24a6d71
	k8s.io/client-go v11.0.0+incompatible
)

replace github.com/spinnaker/spin => github.com/coryb/spin v0.4.1-0.20200402220941-467affe3f2ca
