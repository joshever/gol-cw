package gol

import "sync"

// SDL function
func sdl(w *World, c distributorChannels, sdlDone chan bool, turnComplete chan bool, mutex *sync.Mutex) {
	for {
		select {
		case <-sdlDone:
			return
		case <-turnComplete:
			// Mutex to cover data race for w
			mutex.Lock()
			c.events <- TurnComplete{w.turns}
			mutex.Unlock()
		}
	}
}
