package communicator

import (
	"strconv"

	"../network/bcast"
	"../network/peers"
	"../type-library/lib"
)

const createPort int = 6969
const updatePort int = 6968
const helpPort int = 6967

type updateMsgUDP struct {
	E        lib.Elevator
	SenderID int
	AllCab   map[string][]int
	AllHall  map[string]lib.HallOrder
}

type helpMsgUDP struct {
	ElevID   int
	NeedHelp bool
	AllCab   map[string][]int
	AllHall  map[string]lib.HallOrder
}

func Communicator(ch_rcvCreateMsg chan<- lib.CreateMsg, ch_sendCreateMsg <-chan lib.CreateMsg, ch_sendUpdateMsg <-chan lib.UpdateMsg,
	ch_rcvUpdateMsg chan<- lib.UpdateMsg, localID int, ch_peerUpdate chan<- peers.PeerUpdate, ch_setHelpMode <-chan bool,
	ch_requestHelp <-chan lib.HelpMsg, ch_rcvHelp chan<- lib.HelpMsg, ch_incomingHelpRequest chan<- int, ch_sendHelp <-chan lib.HelpMsg) {

	helpMode := true

	ch_broadcastCreateMsg := make(chan lib.CreateMsg)
	ch_incomingCreateMsg := make(chan lib.CreateMsg)
	ch_broadcastUpdateMsg := make(chan updateMsgUDP)
	ch_incomingUpdateMsg := make(chan updateMsgUDP)
	ch_incomingHelpMsg := make(chan helpMsgUDP)
	ch_broadcastHelpMsg := make(chan helpMsgUDP)
	ch_incomingPeerUpdate := make(chan peers.PeerUpdate)
	ch_peerTxEnable := make(chan bool)

	go peers.Transmitter(15647, strconv.Itoa(localID), ch_peerTxEnable)
	go peers.Receiver(15647, ch_incomingPeerUpdate)

	go bcast.Transmitter(createPort, ch_broadcastCreateMsg)
	go bcast.Receiver(createPort, ch_incomingCreateMsg)

	go bcast.Transmitter(updatePort, ch_broadcastUpdateMsg)
	go bcast.Receiver(updatePort, ch_incomingUpdateMsg)

	go bcast.Transmitter(helpPort, ch_broadcastHelpMsg)
	go bcast.Receiver(helpPort, ch_incomingHelpMsg)

	for {
		if helpMode {
			select {
			case weNeedHelp := <-ch_requestHelp:
				ch_broadcastHelpMsg <- convertHelpToUDP(weNeedHelp)

			case receivedHelpMsg := <-ch_incomingHelpMsg:
				if receivedHelpMsg.ElevID == localID && receivedHelpMsg.NeedHelp == false {
					ch_rcvHelp <- convertHelpFromUDP(receivedHelpMsg)
				}

			case helpMode = <-ch_setHelpMode:
			}
		} else {
			select {
			case outgoingCreateMsg := <-ch_sendCreateMsg:
				ch_broadcastCreateMsg <- outgoingCreateMsg

			case incomingCreateMsg := <-ch_incomingCreateMsg:
				if incomingCreateMsg.SenderID != localID {
					ch_rcvCreateMsg <- lib.DupCreateMsg(incomingCreateMsg)
				}

			case incomingUpdateMsg := <-ch_incomingUpdateMsg:
				if incomingUpdateMsg.SenderID != localID {
					ch_rcvUpdateMsg <- convertUpdateFromUDP(incomingUpdateMsg)
				}

			case outgoingUpdateMsg := <-ch_sendUpdateMsg:
				ch_broadcastUpdateMsg <- convertUpdateToUDP(outgoingUpdateMsg)

			case peerUpdate := <-ch_incomingPeerUpdate:
				ch_peerUpdate <- peerUpdate

			case helpOthers := <-ch_incomingHelpMsg:
				if helpOthers.ElevID != localID && helpOthers.NeedHelp == true {
					ch_incomingHelpRequest <- helpOthers.ElevID

					outgoingHelp := <-ch_sendHelp
					ch_broadcastHelpMsg <- convertHelpToUDP(outgoingHelp)
				}
			case helpMode = <-ch_setHelpMode:
			}
		}
	}
}

//converting functions: json only allows strings as keys for maps.
func convertUpdateToUDP(update lib.UpdateMsg) updateMsgUDP {
	var allCab = make(map[string][]int)
	var allHall = make(map[string]lib.HallOrder)
	for id, orders := range lib.DupCab(update.AllCab) {
		allCab[strconv.Itoa(id)] = orders
	}
	for id, orders := range lib.DupHall(update.AllHall) {
		allHall[strconv.Itoa(id)] = orders
	}
	return updateMsgUDP{E: lib.DupElevator(update.E), SenderID: update.SenderID, AllCab: allCab, AllHall: allHall}
}

func convertUpdateFromUDP(update updateMsgUDP) lib.UpdateMsg {
	var allCab = make(map[int][]int)
	var allHall = make(map[int]lib.HallOrder)
	for id, order := range update.AllCab {
		strID, _ := strconv.Atoi(id)
		allCab[strID] = order
	}
	for id, order := range update.AllHall {
		strID, _ := strconv.Atoi(id)
		allHall[strID] = order
	}
	return lib.UpdateMsg{E: lib.DupElevator(update.E), SenderID: update.SenderID, AllCab: lib.DupCab(allCab), AllHall: lib.DupHall(allHall)}
}

func convertHelpToUDP(help lib.HelpMsg) helpMsgUDP {
	var allCab = make(map[string][]int)
	var allHall = make(map[string]lib.HallOrder)
	for id, order := range lib.DupCab(help.AllCab) {
		allCab[strconv.Itoa(id)] = order
	}
	for id, order := range lib.DupHall(help.AllHall) {
		allHall[strconv.Itoa(id)] = order
	}
	return helpMsgUDP{ElevID: help.ElevID, NeedHelp: help.NeedHelp, AllCab: allCab, AllHall: allHall}
}

func convertHelpFromUDP(help helpMsgUDP) lib.HelpMsg {
	var allCab = make(map[int][]int)
	var allHall = make(map[int]lib.HallOrder)
	for id, order := range help.AllCab {
		strID, _ := strconv.Atoi(id)
		allCab[strID] = order
	}
	for id, order := range help.AllHall {
		strID, _ := strconv.Atoi(id)
		allHall[strID] = order
	}
	return lib.HelpMsg{ElevID: help.ElevID, NeedHelp: help.NeedHelp, AllCab: lib.DupCab(allCab), AllHall: lib.DupHall(allHall)}
}
