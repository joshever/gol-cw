package gol

import (
	"sync"
	"time"
)

// Ticker function
func tick(w *World, c distributorChannels, tickerDone chan bool, pauseTicker chan bool, mutex *sync.Mutex) {
	ticker := time.NewTicker(2 * time.Second)
	for {
		select {
		case <-pauseTicker:
			<-pauseTicker
		case <-tickerDone:
			return
		case <-ticker.C:
			// Mutex to cover data race for w
			mutex.Lock()
			c.events <- AliveCellsCount{w.turns, len(calculateAliveCells(w.world))}
			mutex.Unlock()
		}
	}
}
