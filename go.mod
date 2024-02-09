module github.com/bensallen/node_power

go 1.21.3

require github.com/urfave/cli/v2 v2.25.7

require (
	github.com/andybalholm/brotli v1.0.5 // indirect
	github.com/klauspost/compress v1.16.3 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
)

require (
	github.com/bensallen/nodeset v0.0.0-20231025164848-156f3e4b3fd1
	github.com/bgentry/go-netrc v0.0.0-20140422174119-9fd32a8b3d3d
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/valyala/fasthttp v1.50.0
	github.com/valyala/fastjson v1.6.4
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
)

replace github.com/bgentry/go-netrc => github.com/bensallen/go-netrc v0.0.0-20240208233842-0f11665fc14d
