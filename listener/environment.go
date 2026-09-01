package listener

import (
	"os"
	"strconv"
)

// PortEnvName is the environment variable that carries the listener
// specification.
const PortEnvName = "SERVER_STARTER_PORT"

// GenerationEnvName is the environment variable that carries the generation
// number of the server_starter process. It is incremented every time a new
// worker is started.
const GenerationEnvName = "SERVER_STARTER_GENERATION"

// IsUnderStartServer reports whether the calling process was spawned by the
// start_server supervisor.
func IsUnderStartServer() bool {
	_, ok := os.LookupEnv(GenerationEnvName)
	return ok
}

// Generation returns the generation number of the server_starter process.
// It is incremented every time a new worker is started.
// If the process was not started by server_starter, the second return value
// will be false.
func Generation() (int, bool) {
	genStr, ok := os.LookupEnv(GenerationEnvName)
	if !ok {
		return 0, false
	}
	gen, err := strconv.Atoi(genStr)
	if err != nil {
		return 0, false
	}
	if gen < 0 {
		return 0, false
	}
	return gen, true
}
