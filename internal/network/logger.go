package network

import "log"

type Logger struct{}

func (l Logger) Info(format string, args ...interface{}) {
	log.Printf("[INFO] "+format, args...)
}

func (l Logger) Error(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

func (l Logger) Debug(format string, args ...interface{}) {
	log.Printf("[DEBUG] "+format, args...)
}



