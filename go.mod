module github.com/dock-at-the-yards/shipyard-cli

go 1.26.9

require (
	github.com/dock-at-the-yards/shipyard-core v0.0.0
	github.com/spf13/cobra v1.10.2
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/pelletier/go-toml/v2 v2.4.3 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
)

// The engine is developed in the adjacent shipyard-core repository. Until
// shipyard-core is tagged, resolve it from the local checkout.
replace github.com/dock-at-the-yards/shipyard-core => ../shipyard-core
