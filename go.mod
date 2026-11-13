module github.com/bensallen/node_power

go 1.26.0

require github.com/urfave/cli/v2 v2.27.7

require (
	github.com/klauspost/compress v1.20.1 // indirect
	github.com/molecule-man/go-brrr v1.0.1 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
)

require (
	github.com/bensallen/nodeset v0.0.0-20260928174813-dd1052c7fdcd
	github.com/bgentry/go-netrc v0.0.0-20140422174119-9fd32a8b3d3d
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/valyala/fasthttp v1.74.0
	github.com/valyala/fastjson v1.6.10
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
)

// TODO: repoint at a bensallen/go-netrc pseudo-version once the
// findmachine_index branch is pushed. The local path is here for the
// FindMachine lookup index.
replace github.com/bgentry/go-netrc => github.com/bensallen/go-netrc v0.0.0-20261113211723-644c0042b4b1
