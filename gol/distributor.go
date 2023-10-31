package gol

import (
	"fmt"
	"sync"
	"time"
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

	// Construct file name and trigger IO to fill channel with file bytes
	inputFilename := fmt.Sprintf("%dx%d", p.ImageWidth, p.ImageHeight)
	c.ioCommand <- ioInput
	c.ioFilename <- inputFilename

	// Local turn and world variables
	// world is filled byte by byte from IO input
	turn := 0
	world := createEmptyWorld(p)
	for j := 0; j < p.ImageHeight; j++ {
		for i := 0; i < p.ImageWidth; i++ {
			nextByte := <-c.ioInput
			world[j][i] = nextByte
			if nextByte == ALIVE {
				c.events <- CellFlipped{0, util.Cell{i, j}}
			}
		}
	}

	// Make local mutex, world struct and done channels
	var mutex = sync.Mutex{}
	w := &World{world: world, turns: turn}
	tickerDone := make(chan bool)
	sdlDone := make(chan bool)
	turnComplete := make(chan bool)

	// Run ticker goroutine
	go func(w *World, c distributorChannels, tickerDone chan bool) {
		ticker := time.NewTicker(2 * time.Second)
		for {
			select {
			case <-tickerDone:
				return
			case <-ticker.C:
				// Mutex to cover data race for w
				mutex.Lock()
				c.events <- AliveCellsCount{w.turns, len(calculateAliveCells(w.world))}
				mutex.Unlock()
			}
		}
	}(w, c, tickerDone)

	// Run SDL goroutine
	go func(w *World, c distributorChannels, sdlDone chan bool, turnComplete chan bool) {
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
	}(w, c, sdlDone, turnComplete)

	// Run parallel GOL Turns
	for i := 0; i < p.Turns; i++ {
		old := makeNewWorld(p, world)
		// Sequential if 1 thread
		if p.Threads == 1 {
			world = calculateNextState(p, world, 0, p.ImageHeight)
		} else {
			world = parallel(p, world)
		}
		turn++
		turnComplete <- true
		for j := 0; j < p.ImageHeight; j++ {
			for i := 0; i < p.ImageWidth; i++ {
				if world[j][i] != old[j][i] {
					c.events <- CellFlipped{turn, util.Cell{i, j}}
				}
			}
		}
		// Update w in mutex
		mutex.Lock()
		w.turns = turn
		w.world = world
		mutex.Unlock()
	}

	// Writing PGM file to IO output
	writePgm(p, c, world)

	// Final Turn Complete
	aliveCells := calculateAliveCells(world)
	finalState := FinalTurnComplete{turn, aliveCells}
	c.events <- finalState
	tickerDone <- true
	sdlDone <- true

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

func writePgm(p Params, c distributorChannels, world [][]byte) {
	outputFilename := fmt.Sprintf("%dx%dx%d", p.ImageWidth, p.ImageHeight, p.Turns)
	c.ioCommand <- ioOutput
	c.ioFilename <- outputFilename
	for j := 0; j < p.ImageHeight; j++ {
		for i := 0; i < p.ImageWidth; i++ {
			c.ioOutput <- world[j][i]
		}
	}
}

func calculateNextState(p Params, world [][]byte, startY, endY int) [][]byte {
	// Deep Copy each world to avoid shared memory
	newWorld := makeNewWorld(p, world)
	for j := startY; j < endY; j++ {
		for i := 0; i < p.ImageWidth; i++ {
			// Use un-copied world as operations are read only
			aliveNeighbours := findAliveNeighbours(p, world, j, i)
			if world[j][i] == ALIVE {
				if aliveNeighbours < 2 {
					newWorld[j][i] = DEAD
				} else if aliveNeighbours <= 3 {
					newWorld[j][i] = ALIVE
				} else {
					newWorld[j][i] = DEAD
				}
			} else {
				if aliveNeighbours == 3 {
					newWorld[j][i] = ALIVE
				}
			}
		}
	}
	return newWorld
}

func createEmptyWorld(p Params) [][]byte {
	world := make([][]byte, p.ImageHeight)
	for k := range world {
		world[k] = make([]byte, p.ImageWidth)
	}
	return world
}

func makeNewWorld(p Params, world [][]byte) [][]byte {
	newWorld := createEmptyWorld(p)
	for k := range world {
		copy(newWorld[k], world[k])
	}
	return newWorld
}

func findAliveNeighbours(p Params, world [][]byte, x int, y int) int {
	alive := 0
	for j := -1; j <= 1; j++ {
		for i := -1; i <= 1; i++ {
			if i == 0 && j == 0 {
				continue
			}
			ny, nx := y+j, x+i
			if ny == p.ImageHeight {
				ny = 0
			} else if ny < 0 {
				ny = p.ImageHeight - 1
			}
			if nx < 0 {
				nx = p.ImageWidth - 1
			} else if nx == p.ImageWidth {
				nx = 0
			}
			if world[nx][ny] == ALIVE {
				alive += 1
			}
		}
	}
	return alive
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

func parallel(p Params, world [][]byte) [][]byte {
	var newPixelData [][]byte
	newHeight := p.ImageHeight / p.Threads
	// List of channels for each thread
	channels := make([]chan [][]byte, p.Threads)
	for i := 0; i < p.Threads; i++ {
		channels[i] = make(chan [][]byte)
		// Cover gaps missed due to rounding in last strip
		if i == p.Threads-1 {
			go worker(p, i*newHeight, p.ImageHeight, world, channels[i])
		} else {
			go worker(p, i*newHeight, (i+1)*newHeight, world, channels[i])
		}
	}
	for i := 0; i < p.Threads; i++ {
		// Read from specific channels in order to reassemble
		newPixelData = append(newPixelData, <-channels[i]...)
	}
	return newPixelData
}

func worker(p Params, startY, endY int, world [][]byte, out chan<- [][]uint8) {
	// Pass whole world which is then deep copied
	returned := calculateNextState(p, world, startY, endY)
	// Slice output into correct strip
	returned = returned[startY:endY]
	out <- returned
}
