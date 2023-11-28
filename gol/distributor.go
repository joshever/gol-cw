package gol

import (
	"fmt"
	"sync"
	"uk.ac.bris.cs/gameoflife/util"
)

const ALIVE = byte(255)
const DEAD = byte(0)

type World struct {
	world [][]byte
	turns int
}

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	keys       <-chan rune
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	// Setup
	turn := 0
	world := setup(p, c)

	// Make local mutex, World struct and channels
	var mutex = sync.Mutex{}
	w := &World{world: world, turns: turn}
	tickerDone := make(chan bool)
	keyPressesDone := make(chan bool)
	pauseDistributor := make(chan bool)
	pauseTicker := make(chan bool)

	// run ticker goroutine
	go ticker(w, c, tickerDone, pauseTicker, &mutex)
	// run presses goroutine
	go keyPresses(p, w, c, keyPressesDone, pauseDistributor, pauseTicker, &mutex)

	// Run parallel GOL Turns
	for i := 0; i < p.Turns; i++ {
		select {
		case <-pauseDistributor:
			<-pauseDistributor
		default:
			// Make copy of world
			old := makeNewWorld(p, world, 0, p.ImageHeight)
			// Call update state goroutine
			world = next(p, world)
			turn++
			// Cell flipped event
			for j := 0; j < p.ImageHeight; j++ {
				for k := 0; k < p.ImageWidth; k++ {
					if world[j][k] != old[j][k] {
						c.events <- CellFlipped{turn, util.Cell{k, j}}
					}
				}
			}
			// Update w in mutex
			mutex.Lock()
			w.turns = turn
			w.world = world
			c.events <- TurnComplete{w.turns}
			mutex.Unlock()
		}
	}

	// Writing PGM file to IO output
	mutex.Lock()
	writePgm(p, c, w)
	mutex.Unlock()

	// Final Turn Complete
	finalState := FinalTurnComplete{turn, calculateAliveCells(world)}
	c.events <- finalState
	// Terminate go routines
	tickerDone <- true
	keyPressesDone <- true
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	c.events <- StateChange{turn, Quitting}
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

// Declare functions used in goroutines
func writePgm(p Params, c distributorChannels, w *World) {
	outputFilename := fmt.Sprintf("%dx%dx%d", p.ImageWidth, p.ImageHeight, w.turns)
	c.ioCommand <- ioOutput
	c.ioFilename <- outputFilename
	for j := 0; j < p.ImageHeight; j++ {
		for i := 0; i < p.ImageWidth; i++ {
			c.ioOutput <- w.world[j][i]
		}
	}
}

func calculateAliveCells(world [][]byte) []util.Cell {
	var cells = []util.Cell{}
	for j := range world {
		for i := range world[0] {
			if world[j][i] == byte(255) {
				cells = append(cells, util.Cell{i, j})
			}
		}
	}
	return cells
}

func setup(p Params, c distributorChannels) [][]byte {
	// Construct file name and trigger IO to fill channel with file bytes
	inputFilename := fmt.Sprintf("%dx%d", p.ImageWidth, p.ImageHeight)
	c.ioCommand <- ioInput
	c.ioFilename <- inputFilename

	// Local turn and world variables
	// world is filled byte by byte from IO input
	world := createEmptyWorld(p, 0, p.ImageHeight)
	for j := 0; j < p.ImageHeight; j++ {
		for i := 0; i < p.ImageWidth; i++ {
			nextByte := <-c.ioInput
			world[j][i] = nextByte
			if nextByte == ALIVE {
				c.events <- CellFlipped{0, util.Cell{i, j}}
			}
		}
	}
	return world
}
