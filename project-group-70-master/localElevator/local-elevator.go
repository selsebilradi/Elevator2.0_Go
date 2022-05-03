package localElevator

import (
	"../type-library/lib"
	"./driver-go/elevio"
	"./fsm"
)

//Does not send anything to channel at the moment
func LocalElevator(ch_executeOrder <-chan lib.ButtonEvent, ch_localStateUpdate chan<- lib.Elevator) {

	ch_drvFloors := make(chan int)
	ch_drvObstr := make(chan bool)

	go elevio.PollFloorSensor(ch_drvFloors)
	go elevio.PollObstructionSwitch(ch_drvObstr)

	go fsm.FSM(ch_executeOrder, ch_drvFloors, ch_drvObstr, ch_localStateUpdate)

	for {

	}
}
