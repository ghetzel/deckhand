module github.com/ghetzel/deckhand

go 1.14

require (
	github.com/ghetzel/cli v1.17.0
	github.com/ghetzel/dataclutch v0.0.5
	github.com/ghetzel/diecast v1.19.19
	github.com/ghetzel/go-defaults v1.2.0
	github.com/ghetzel/go-stockutil v1.9.4
	github.com/ghetzel/sysfact v0.7.2
	github.com/magicmonkey/go-streamdeck v0.0.1-alpha
	github.com/radovskyb/watcher v1.0.7
	github.com/tdewolff/canvas v0.0.0-20201231005725-5d279dbb51d6
	gopkg.in/yaml.v2 v2.3.0
)

replace github.com/tdewolff/canvas v0.0.0-20201231005725-5d279dbb51d6 => github.com/ghetzel/canvas v0.0.0-20210104211054-1e57a21026a1

replace github.com/ghetzel/dataclutch v0.0.5 => ../dataclutch

// replace github.com/ghetzel/diecast v1.19.19 => ../diecast
