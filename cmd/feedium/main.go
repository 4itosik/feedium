package main

import "feedium/internal/platform/logger"

func main() {
	log := logger.Init()
	log.Info("Feedium is starting")
}
