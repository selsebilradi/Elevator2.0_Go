package distributor

import (
	"strconv"
	"time"

	"../localElevator/driver-go/elevio"
	"../network/peers"
	"../type-library/lib"
	"./cost"
)

const _TIMEOUT = 20
const _MAX_INT = int(^uint(0) >> 1)
const _RESEND_TIMEOUT = 100
const _INVAL_ID = -1

var dist_peerStates = make(map[int]lib.Elevator)
var dist_allCab = make(map[int][]int)
var dist_allHall = make(map[int]lib.HallOrder)
var dist_syncedOrders = make([][lib.NumButtons]lib.CreateMsg, elevio.NumFloors())
var myID int = 0

func Distributor(id int, ch_executeOrder chan<- lib.ButtonEvent, ch_localStateUpdate <-chan lib.Elevator, ch_sendCreateMsg chan<- lib.CreateMsg,
	ch_rcvCreateMsg <-chan lib.CreateMsg, ch_rcvUpdateMsg <-chan lib.UpdateMsg, ch_sendUpdateMsg chan<- lib.UpdateMsg, getPeerUpdateCh <-chan peers.PeerUpdate,
	ch_requestHelp chan<- lib.HelpMsg, ch_rcvHelp <-chan lib.HelpMsg, ch_setHelpMode chan<- bool, ch_incomingHelpRequest <-chan int, ch_sendHelp chan<- lib.HelpMsg) {
	myID = id
	dist_peerStates[myID] = elevio.ElevatorInitializer()

	//Start program by asking potential other elevators for help
	ch_setHelpMode <- true
	dist_allCab, dist_allHall = requestHelp(myID, ch_requestHelp, ch_rcvHelp)
	feedCabsToLocal(ch_executeOrder)
	ch_setHelpMode <- false

	ch_hwButtons := make(chan lib.ButtonEvent)
	ch_timedOutOrders := make(chan lib.ButtonEvent)

	go elevio.PollButtons(ch_hwButtons)
	go timeoutChecker(ch_timedOutOrders, ch_sendCreateMsg)
	go cabOrderFallback(ch_executeOrder)

	for {
		updateLights()
		select {
		case buttonpress := <-ch_hwButtons:
			distributeOrder(buttonpress, ch_executeOrder, ch_sendCreateMsg)
		case StateUpdate := <-ch_localStateUpdate:
			localStateUpdate(StateUpdate)
			ch_sendUpdateMsg <- myCurrentData()
		case receivedCreateMsg := <-ch_rcvCreateMsg:
			createOrder(receivedCreateMsg)
			if receivedCreateMsg.ReceiverID == myID && receivedCreateMsg.SenderID != myID {
				checkAlreadyOnFloor(receivedCreateMsg.Order)
				ch_executeOrder <- receivedCreateMsg.Order
			}
			ch_sendUpdateMsg <- myCurrentData()
		case receivedUpdateMsg := <-ch_rcvUpdateMsg:
			newOrders := interpretUpdate(receivedUpdateMsg, ch_executeOrder)
			if newOrders {
				ch_sendUpdateMsg <- myCurrentData()
			}
		case peerUpdate := <-getPeerUpdateCh:
			for lost := range peerUpdate.Lost {
				lostID, _ := strconv.Atoi(peerUpdate.Lost[lost])
				if lostID != myID {
					delete(dist_peerStates, lostID)
				}
			}
			if len(peerUpdate.New) > 0 {
				idInt, _ := strconv.Atoi(peerUpdate.New)
				if len(dist_allCab[idInt]) == 0 {
					dist_allCab[idInt] = cabInit()
				}
				if len(dist_peerStates) == 1 {
					ch_setHelpMode <- true
					helpCab, helpHall := requestHelp(myID, ch_requestHelp, ch_rcvHelp)
					synchronizeOrderTables(helpCab, helpHall)
					ch_setHelpMode <- false
				}
				ch_sendUpdateMsg <- myCurrentData()
			}
		case timedOut := <-ch_timedOutOrders:
			ch_executeOrder <- timedOut
		case needsHelp := <-ch_incomingHelpRequest:
			ch_sendHelp <- lib.HelpMsg{
				ElevID:   needsHelp,
				NeedHelp: false,
				AllCab:   lib.DupCab(dist_allCab),
				AllHall:  lib.DupHall(dist_allHall),
			}
		}
	}
}

func distributeOrder(hwOrder lib.ButtonEvent, ch_executeOrder chan<- lib.ButtonEvent, ch_sendCreateMsg chan<- lib.CreateMsg) {
	var recvID int
	var minCost int
	//Special case: Cab order used as "open door-button at floor"
	if hwOrder.Button == lib.BT_Cab {
		if hwOrder.Floor == dist_peerStates[myID].Floor && dist_peerStates[myID].Behaviour != lib.EB_Moving {
			ch_executeOrder <- hwOrder
			return
		}
	}
	if orderAlreadyInSystem(hwOrder) {
		return
	}
	//Single-elevator mode
	if len(dist_peerStates) == 1 {
		if hwOrder.Button == lib.BT_Cab {
			ch_executeOrder <- hwOrder
			dist_allCab[myID][hwOrder.Floor] = 1
		}
	} else {
		if hwOrder.Button != lib.BT_Cab {
			minCost, recvID = findBestElevator(hwOrder)
			if minCost < 0 {
				return //Order is already in execution
			}
		} else {
			recvID = myID
		}

		msg := lib.CreateMsg{
			Order:      hwOrder,
			ReceiverID: recvID,
			SenderID:   myID,
			Timestamp:  time.Now(),
		}
		dist_syncedOrders[hwOrder.Floor][int(hwOrder.Button)] = lib.DupCreateMsg(msg)
		ch_sendCreateMsg <- msg
	}
}

func orderAlreadyInSystem(o lib.ButtonEvent) bool {
	switch o.Button {
	case lib.BT_Cab:
		if dist_allCab[myID][o.Floor] == 1 {
			return true
		}
	case lib.BT_HallUp:
		if dist_allHall[o.Floor].UpID != _INVAL_ID {
			return true
		}
	case lib.BT_HallDown:
		if dist_allHall[o.Floor].DownID != _INVAL_ID {
			return true
		}
	}
	return expectingOrder(o)
}

func findBestElevator(o lib.ButtonEvent) (minCost, bestID int) {
	minCost = _MAX_INT
	for id, elev := range dist_peerStates {
		elevcost := cost.Cost(elev, o.Button, o.Floor) + id
		if elevcost < minCost {
			bestID = id
			minCost = elevcost
		}
	}
	return
}

func expectingOrder(o lib.ButtonEvent) bool {
	return !dist_syncedOrders[o.Floor][int(o.Button)].Timestamp.IsZero()
}

func localStateUpdate(e lib.Elevator) {
	for floor := range e.Orders {
		for button := range e.Orders[floor] {
			if e.Orders[floor][button] == 0 && dist_peerStates[myID].Orders[floor][button] == 1 {
				order := lib.ButtonEvent{Floor: floor, Button: lib.ButtonType(button)}
				//Update all states according to which type of order has been cleared
				if order.Button == lib.BT_Cab {
					dist_allCab[myID][order.Floor] = 0
				} else if order.Button == lib.BT_HallUp {
					dist_allHall[order.Floor] = lib.HallOrder{
						UpID:        _INVAL_ID,
						UpTimeout:   time.Time{},
						DownID:      dist_allHall[order.Floor].DownID,
						DownTimeout: dist_allHall[order.Floor].DownTimeout,
					}
				} else {
					dist_allHall[order.Floor] = lib.HallOrder{
						UpID:        dist_allHall[order.Floor].UpID,
						UpTimeout:   dist_allHall[order.Floor].UpTimeout,
						DownID:      _INVAL_ID,
						DownTimeout: time.Time{},
					}
				}
			}
		}
	}
	//Update our state for syncing with other elevators
	dist_peerStates[myID] = lib.DupElevator(e)
}

func myCurrentData() lib.UpdateMsg {
	return lib.UpdateMsg{
		E:        lib.DupElevator(dist_peerStates[myID]),
		SenderID: myID,
		AllCab:   lib.DupCab(dist_allCab),
		AllHall:  lib.DupHall(dist_allHall),
	}
}

//When a createMsg has been sent from another elevator
func createOrder(d lib.CreateMsg) {
	switch d.Order.Button {
	case lib.BT_HallUp:
		dist_allHall[d.Order.Floor] = lib.HallOrder{
			UpID:        d.ReceiverID,
			UpTimeout:   d.Timestamp,
			DownID:      dist_allHall[d.Order.Floor].DownID,
			DownTimeout: dist_allHall[d.Order.Floor].DownTimeout,
		}
		break

	case lib.BT_HallDown:
		dist_allHall[d.Order.Floor] = lib.HallOrder{
			UpID:        dist_allHall[d.Order.Floor].UpID,
			UpTimeout:   dist_allHall[d.Order.Floor].UpTimeout,
			DownID:      d.ReceiverID,
			DownTimeout: d.Timestamp,
		}
		break

	case lib.BT_Cab:
		dist_allCab[d.ReceiverID][d.Order.Floor] = 1
		break
	}
}

func checkAlreadyOnFloor(o lib.ButtonEvent) bool {
	if o.Floor == dist_peerStates[myID].Floor {
		if dist_peerStates[myID].Behaviour == lib.EB_DoorOpen || dist_peerStates[myID].Behaviour == lib.EB_Idle {
			e := lib.DupElevator(dist_peerStates[myID])
			e.Orders[o.Floor][int(o.Button)] = 1
			dist_peerStates[myID] = e
			return true
		}
	}
	return false
}

func interpretUpdate(u lib.UpdateMsg, ch_executeOrder chan<- lib.ButtonEvent) bool {
	newOrdersInSystem := false
	updatedHallOrders := lib.DupHall(dist_allHall)
	updatedCabOrders := lib.DupCab(u.AllCab)
	updatedCabOrders[myID] = dist_allCab[myID]
	dist_peerStates[u.SenderID] = lib.Elevator{
		Floor:     u.E.Floor,
		Direction: u.E.Direction,
		Orders:    u.E.Orders,
		Behaviour: u.E.Behaviour,
		Cfg:       u.E.Cfg,
	}

	for floor := range u.AllCab[myID] {
		order := lib.ButtonEvent{Floor: floor, Button: lib.BT_Cab}
		if u.AllCab[myID][floor] == 1 && dist_allCab[myID][floor] == 0 && expectingOrder(order) { //if I have new inside
			dist_syncedOrders[floor][int(lib.BT_Cab)].Timestamp = time.Time{}
			if !checkAlreadyOnFloor(order) {
				updatedCabOrders[myID][floor] = 1
			}
			ch_executeOrder <- order
			newOrdersInSystem = true
		}
	}

	for floor, hallOrder := range u.AllHall {
		order := lib.ButtonEvent{Floor: floor, Button: lib.BT_HallDown}
		if hallOrder.DownID == _INVAL_ID && dist_allHall[floor].DownID != _INVAL_ID {
			if dist_allHall[floor].DownID == u.SenderID || isTimedOutHall(dist_allHall[floor], lib.BT_HallDown) {
				updatedHallOrders[floor] = lib.HallOrder{
					UpID:        hallOrder.UpID,
					UpTimeout:   hallOrder.UpTimeout,
					DownID:      _INVAL_ID,
					DownTimeout: time.Time{},
				}
				newOrdersInSystem = true
			}
		} else if hallOrder.DownID != _INVAL_ID && dist_allHall[floor].DownID == _INVAL_ID && expectingOrder(order) {
			dist_syncedOrders[floor][int(lib.BT_HallDown)].Timestamp = time.Time{}
			updatedHallOrders[floor] = lib.HallOrder{
				UpID:        dist_allHall[floor].UpID,
				UpTimeout:   dist_allHall[floor].UpTimeout,
				DownID:      u.AllHall[floor].DownID,
				DownTimeout: u.AllHall[floor].DownTimeout,
			}
			if hallOrder.DownID == myID {
				checkAlreadyOnFloor(order)
				ch_executeOrder <- order
			}
			newOrdersInSystem = true
		}

		order = lib.ButtonEvent{Floor: floor, Button: lib.BT_HallUp}
		if hallOrder.UpID == _INVAL_ID && dist_allHall[floor].UpID != _INVAL_ID {
			if dist_allHall[floor].UpID == u.SenderID || isTimedOutHall(dist_allHall[floor], lib.BT_HallUp) {
				updatedHallOrders[floor] = lib.HallOrder{
					UpID:        _INVAL_ID,
					UpTimeout:   time.Time{},
					DownID:      hallOrder.DownID,
					DownTimeout: hallOrder.DownTimeout,
				}
				newOrdersInSystem = true
			}
		} else if hallOrder.UpID != _INVAL_ID && dist_allHall[floor].UpID == _INVAL_ID && expectingOrder(order) {
			dist_syncedOrders[floor][int(lib.BT_HallUp)].Timestamp = time.Time{}
			updatedHallOrders[floor] = lib.HallOrder{
				UpID:        u.AllHall[floor].UpID,
				UpTimeout:   u.AllHall[floor].UpTimeout,
				DownID:      dist_allHall[floor].DownID,
				DownTimeout: dist_allHall[floor].DownTimeout,
			}
			if hallOrder.UpID == myID {
				checkAlreadyOnFloor(order)
				ch_executeOrder <- order
			}
			newOrdersInSystem = true
		}
	}
	dist_allHall = lib.DupHall(updatedHallOrders)
	dist_allCab = lib.DupCab(updatedCabOrders)
	return newOrdersInSystem
}

func synchronizeOrderTables(updatedCabOrders map[int][]int, updatedHallOrders map[int]lib.HallOrder) {
	for id, orders := range updatedCabOrders {
		for floor := range orders {
			_, exists := dist_allCab[id]
			if exists {
				if (updatedCabOrders[id][floor] == 1 || dist_allCab[id][floor] == 1) && id != myID {
					dist_allCab[id][floor] = 1
				}
			} else {
				dist_allCab[id] = orders
			}
		}
	}
	for floor, orders := range dist_allHall {
		if orders.DownID == _INVAL_ID && updatedHallOrders[floor].DownID != myID {
			dist_allHall[floor] = lib.HallOrder{
				UpID:        orders.UpID,
				UpTimeout:   orders.UpTimeout,
				DownID:      updatedHallOrders[floor].DownID,
				DownTimeout: updatedHallOrders[floor].DownTimeout,
			}
		}
		if orders.UpID == _INVAL_ID && updatedHallOrders[floor].UpID != myID {
			dist_allHall[floor] = lib.HallOrder{
				UpID:        updatedHallOrders[floor].UpID,
				UpTimeout:   updatedHallOrders[floor].UpTimeout,
				DownID:      orders.DownID,
				DownTimeout: orders.DownTimeout,
			}
		}
	}
}

func requestHelp(localID int, ch_requestHelp chan<- lib.HelpMsg, ch_rcvHelp <-chan lib.HelpMsg) (map[int][]int, map[int]lib.HallOrder) {
	helpHall := allHallInit()
	helpCab := map[int][]int{}
	ch_requestHelp <- lib.HelpMsg{
		ElevID:   localID,
		NeedHelp: true,
		AllCab:   map[int][]int{},
		AllHall:  map[int]lib.HallOrder{},
	}

	timer := time.NewTimer(50 * time.Millisecond)

	for i := 0; i < 5; i++ {
		select {
		case help := <-ch_rcvHelp:
			helpCab = help.AllCab
			helpHall = help.AllHall
		case <-timer.C:
			ch_requestHelp <- lib.HelpMsg{
				ElevID:   localID,
				NeedHelp: true,
				AllCab:   map[int][]int{},
				AllHall:  map[int]lib.HallOrder{},
			}
			timer.Reset(50 * time.Millisecond)
		}
	}
	if len(helpCab[myID]) == 0 {
		helpCab[myID] = cabInit()
	}
	return helpCab, helpHall
}

func isValidHall(floor int, button lib.ButtonType) bool {
	if floor < 0 {
		return false
	} else if floor >= elevio.NumFloors() {
		return false
	} else if floor == 0 && button == lib.BT_HallDown {
		return false
	} else if floor == elevio.NumFloors()-1 && button == lib.BT_HallUp {
		return false
	}
	return true
}

func updateLights() {
	for floor := 0; floor < elevio.NumFloors(); floor++ {
		if dist_allHall[floor].UpID != _INVAL_ID {
			elevio.SetButtonLamp(lib.BT_HallUp, floor, true)
		} else {
			elevio.SetButtonLamp(lib.BT_HallUp, floor, false)
		}
		if dist_allHall[floor].DownID != _INVAL_ID {
			elevio.SetButtonLamp(lib.BT_HallDown, floor, true)
		} else {
			elevio.SetButtonLamp(lib.BT_HallDown, floor, false)
		}
	}
	for floor := range dist_allCab[myID] {
		elevio.SetButtonLamp(lib.BT_Cab, floor, lib.ToBool(dist_allCab[myID][floor]))
	}
}

func feedCabsToLocal(ch_executeOrder chan<- lib.ButtonEvent) {
	for i := 0; i < elevio.NumFloors(); i++ {
		if dist_allCab[myID][i] == 1 {
			ch_executeOrder <- lib.ButtonEvent{Floor: i, Button: lib.BT_Cab}
		}
	}
}

func cabInit() []int {
	cab := []int{}
	for i := 0; i < elevio.NumFloors(); i++ {
		cab = append(cab, 0)
	}
	return cab
}

func allHallInit() map[int]lib.HallOrder {
	emptyHalls := make(map[int]lib.HallOrder)
	for i := 0; i < elevio.NumFloors(); i++ {
		emptyHalls[i] = lib.HallOrder{
			UpID:        _INVAL_ID,
			UpTimeout:   time.Time{},
			DownID:      _INVAL_ID,
			DownTimeout: time.Time{},
		}
	}
	return emptyHalls
}

func timeoutChecker(ch_timedOutOrders chan<- lib.ButtonEvent, ch_sendCreateMsg chan<- lib.CreateMsg) {
	for {
		time.Sleep(1000 * time.Millisecond)
		for floor := 0; floor < elevio.NumFloors(); floor++ {
			for btn := 0; btn < lib.NumButtons; btn++ {
				order := lib.ButtonEvent{Floor: floor, Button: lib.ButtonType(btn)}
				//Resend any timed out messages:
				if shouldResendMsg(order) {
					_, peerExists := dist_peerStates[dist_syncedOrders[floor][btn].ReceiverID]
					if !peerExists {
						_, redistributeTo := findBestElevator(order)
						dist_syncedOrders[floor][btn].ReceiverID = redistributeTo
					}
					switch btn {
					case int(lib.BT_Cab):
						if dist_allCab[myID][floor] == 1 {
							dist_syncedOrders[floor][btn].Timestamp = time.Time{}
						} else {
							dist_syncedOrders[floor][int(btn)].Timestamp = time.Now()
							ch_sendCreateMsg <- lib.DupCreateMsg(dist_syncedOrders[floor][btn])
						}
					case int(lib.BT_HallDown):
						if dist_allHall[floor].DownID != _INVAL_ID {
							dist_syncedOrders[floor][btn].Timestamp = time.Time{}
						} else {
							dist_syncedOrders[floor][int(btn)].Timestamp = time.Now()
							ch_sendCreateMsg <- lib.DupCreateMsg(dist_syncedOrders[floor][btn])
						}
					case int(lib.BT_HallUp):
						if dist_allHall[floor].UpID != _INVAL_ID {
							dist_syncedOrders[floor][btn].Timestamp = time.Time{}
						} else {
							dist_syncedOrders[floor][btn].Timestamp = time.Now()
							ch_sendCreateMsg <- lib.DupCreateMsg(dist_syncedOrders[floor][btn])
						}
					}
				}
			}
			//Take all timed out hall orders ourselves.
			if isValidHall(floor, lib.BT_HallUp) && dist_allHall[floor].UpID != _INVAL_ID && !dist_allHall[floor].UpTimeout.IsZero() {
				if isTimedOutHall(dist_allHall[floor], lib.BT_HallUp) {
					ch_timedOutOrders <- lib.ButtonEvent{Floor: floor, Button: lib.BT_HallUp}
				}
			}
			if isValidHall(floor, lib.BT_HallDown) && dist_allHall[floor].DownID != _INVAL_ID && !dist_allHall[floor].DownTimeout.IsZero() {
				if isTimedOutHall(dist_allHall[floor], lib.BT_HallDown) {
					ch_timedOutOrders <- lib.ButtonEvent{Floor: floor, Button: lib.BT_HallDown}
				}
			}
		}
	}
}

func shouldResendMsg(o lib.ButtonEvent) bool {
	return (time.Now().After(dist_syncedOrders[o.Floor][int(o.Button)].Timestamp.Add(_RESEND_TIMEOUT*time.Millisecond)) && expectingOrder(o) && dist_syncedOrders[o.Floor][int(o.Button)].ReceiverID != _INVAL_ID)
}

func isTimedOutHall(o lib.HallOrder, t lib.ButtonType) bool {
	timeout := time.Time{}
	switch t {
	case lib.BT_HallUp:
		timeout = o.UpTimeout
		break

	case lib.BT_HallDown:
		timeout = o.DownTimeout
		break
	}
	return time.Now().After(timeout.Add(_TIMEOUT * time.Second))
}

func cabOrderFallback(ch_executeOrder chan<- lib.ButtonEvent) {
	for {
		if dist_peerStates[myID].Behaviour == lib.EB_Idle {
			for floor := 0; floor < elevio.NumFloors(); floor++ {
				if dist_allCab[myID][floor] == 1 { //If we have a cab order but are still idle, we re-take it
					order := lib.ButtonEvent{Floor: floor, Button: lib.BT_Cab}
					checkAlreadyOnFloor(order)
					ch_executeOrder <- order
				}
				if dist_allHall[floor].UpID == myID {
					order := lib.ButtonEvent{Floor: floor, Button: lib.BT_HallUp}
					checkAlreadyOnFloor(order)
					ch_executeOrder <- order
				}
				if dist_allHall[floor].DownID == myID {
					order := lib.ButtonEvent{Floor: floor, Button: lib.BT_HallDown}
					checkAlreadyOnFloor(order)
					ch_executeOrder <- order
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
}
