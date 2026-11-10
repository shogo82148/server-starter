module github.com/shogo82148/server-starter

go 1.26.0

toolchain go1.27.3

require github.com/shogo82148/server-starter/listener v1.0.0

require golang.org/x/sys v0.50.0

replace github.com/shogo82148/server-starter/listener => ./listener
