package main

import (
	"strconv"

	"os"

	"./communicator"
	"./distributor"
	"./localElevator"
	"./localElevator/driver-go/elevio"
	"./network/peers"
	"./type-library/lib"
)

func main() {
	//Optional arguments when starting program are 'ID' and 'Floor'
	strID := "57"
	numFloors := 4
	if len(os.Args) > 1 {
		strID = os.Args[1]
		if len(os.Args) > 2 {
			numFloors, _ = strconv.Atoi(os.Args[2])
		}
	}
	port := "156" + strID
	elevio.Init("localhost:"+port, numFloors)

	ch_sendCreateMsg := make(chan lib.CreateMsg)
	ch_rcvCreateMsg := make(chan lib.CreateMsg)
	ch_sendUpdateMsg := make(chan lib.UpdateMsg, 1024)
	ch_rcvUpdateMsg := make(chan lib.UpdateMsg, 1024)
	ch_executeOrder := make(chan lib.ButtonEvent, 1024)
	ch_localStateUpdate := make(chan lib.Elevator, 1024)
	ch_peerUpdate := make(chan peers.PeerUpdate, 1024)
	ch_requestHelp := make(chan lib.HelpMsg, 1024)
	ch_rcvHelp := make(chan lib.HelpMsg, 1024)
	ch_setHelpMode := make(chan bool)
	ch_incomingHelpRequest := make(chan int)
	ch_sendHelp := make(chan lib.HelpMsg)

	id, _ := strconv.Atoi(strID)

	go distributor.Distributor(id, ch_executeOrder, ch_localStateUpdate, ch_sendCreateMsg, ch_rcvCreateMsg, ch_rcvUpdateMsg, ch_sendUpdateMsg, ch_peerUpdate, ch_requestHelp, ch_rcvHelp, ch_setHelpMode, ch_incomingHelpRequest, ch_sendHelp)
	go localElevator.LocalElevator(ch_executeOrder, ch_localStateUpdate)
	go communicator.Communicator(ch_rcvCreateMsg, ch_sendCreateMsg, ch_sendUpdateMsg, ch_rcvUpdateMsg, id, ch_peerUpdate, ch_setHelpMode, ch_requestHelp, ch_rcvHelp, ch_incomingHelpRequest, ch_sendHelp) //Fiks channelnavn

	for {

	}
}
