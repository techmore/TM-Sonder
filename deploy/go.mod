// The deploy script is bash; these tests drive it as a black box from Go so
// `go test ./...` in CI covers it without needing a second test runner.
module tm-sonder/deploy

go 1.25
