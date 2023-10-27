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

	filename := fmt.Sprintf("%dx%d", p.ImageWidth, p.ImageHeight) + ".pgm"
	c.ioFilename <- filename
	c.ioCommand <- ioInput

	turn := 0

	// TODO: Execute all turns of the Game of Life.

	// TODO: Report the final state using FinalTurnCompleteEvent.

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

func calculateNextState(p Params, world [][]byte) [][]byte {
	newWorld := makeNewWorld(p, world)
	for j := range world {
		for i := range world[0] {
			if newWorld[j][i] == ALIVE {
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
