package gol

import (
	"fmt"
	"uk.ac.bris.cs/gameoflife/util"
)

const ALIVE = byte(255)
const DEAD = byte(0)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	// TODO: Create a 2D slice to store the world.

	filename := fmt.Sprintf("%dx%d", p.ImageWidth, p.ImageHeight)
	c.ioCommand <- ioInput
	c.ioFilename <- filename

	turn := 0

	World := createEmptyWorld(p)
	for j := 0; j < p.ImageHeight; j++ {
		for i := 0; i < p.ImageWidth; i++ {
			nextByte := <-c.ioInput
			World[j][i] = nextByte
		}
	}

	// TODO: Execute all turns of the Game of Life.

	for i := 0; i < p.Turns; i++ {
		World = filter(p, World)
		turn++
	}

	// TODO: Report the final state using FinalTurnCompleteEvent.
	aliveCells := calculateAliveCells(p, World)
	finalState := FinalTurnComplete{turn, aliveCells}
	c.events <- finalState

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

func calculateNextState(p Params, world [][]byte, startY, endY int) [][]byte {
	newWorld := makeNewWorld(p, world)
	for j := startY; j < endY; j++ {
		for i := range world[0] {
			if world[j][i] == ALIVE {
				if findAliveNeighbours(p, world, j, i) < 2 {
					newWorld[j][i] = DEAD
				} else if findAliveNeighbours(p, world, j, i) <= 3 {
					continue
				} else {
					newWorld[j][i] = DEAD
				}
			} else {
				if findAliveNeighbours(p, world, j, i) == 3 {
					newWorld[j][i] = ALIVE
				}
			}
		}
	}
	return newWorld
}

func createEmptyWorld(p Params) [][]byte {
	World := make([][]byte, p.ImageHeight)
	for k := range World {
		World[k] = make([]byte, p.ImageWidth)
	}
	return World
}

func makeNewWorld(p Params, world [][]byte) [][]byte {
	newWorld := make([][]byte, p.ImageHeight)
	for k := range world {
		newWorld[k] = make([]byte, p.ImageWidth)
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

func calculateAliveCells(p Params, world [][]byte) []util.Cell {
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

func filter(p Params, world [][]byte) [][]byte {

	var newPixelData [][]byte
	newHeight := p.ImageHeight / p.Threads
	// List of channels for each thread
	channels := make([]chan [][]byte, p.Threads)
	for i := 0; i < p.Threads; i++ {
		channels[i] = make(chan [][]byte)
		if i == p.Threads - 1{
			go worker(p, i*newHeight, p.ImageHeight, world, channels[i])
		} else{ 
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
	returned := calculateNextState(p, world, startY, endY)
	returned = returned[startY:endY]
	out <- returned
}
