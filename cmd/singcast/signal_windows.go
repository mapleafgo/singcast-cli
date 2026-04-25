package main

import (
	"os"
)

func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

func isReloadSignal(_ os.Signal) bool {
	return false
}
