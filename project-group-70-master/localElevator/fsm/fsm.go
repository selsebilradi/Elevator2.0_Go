package fsm

import (
	"time"

	"../../type-library/lib"
	"../driver-go/elevio"
	"../op"
)

const _MOTOR_TIMEOUT = 10
const _IDLE_UPDATE_FREQUENCY = 4

func FSM(ch_drvButtons <-chan lib.ButtonEvent, ch_drvFloors <-chan int, ch_drvObstr <-chan bool, ch_localstateUpdate chan<- lib.Elevator) {
	e := elevio.ElevatorInitializer()
	select {
	case f := <-ch_drvFloors:
		e.Floor = f
	default:
		e = onInitBetweenFloors(e)
	}
	timer := time.NewTimer(time.Duration(e.Cfg.DoorOpenDuration_s) * time.Second)
	timer.Stop()
	motorTimeout := time.NewTimer(_MOTOR_TIMEOUT * time.Second)
	idleTimeout := time.NewTimer(_IDLE_UPDATE_FREQUENCY * time.Second)
	var prev int = -2
	ch_localstateUpdate <- lib.DupElevator(e)

	for {
		select {
		case event := <-ch_drvButtons:
			e, timer = onRequestButtonPress(event.Floor, event.Button, timer, e)
			ch_localstateUpdate <- lib.DupElevator(e)

		case floor := <-ch_drvFloors:
			if floor != -1 && floor != prev {
				e, timer = onFloorArrival(floor, e, timer)
				ch_localstateUpdate <- lib.DupElevator(e)
				motorTimeout.Reset(_MOTOR_TIMEOUT * time.Second)
			}
			prev = floor

		case obstructed := <-ch_drvObstr:
			e, timer = onObstruction(e, timer, obstructed)
			motorTimeout.Reset(_MOTOR_TIMEOUT * time.Second)
			ch_localstateUpdate <- lib.DupElevator(e)

		case <-timer.C:
			e = onDoorTimeout(e)
			motorTimeout.Reset(_MOTOR_TIMEOUT * time.Second)
			ch_localstateUpdate <- lib.DupElevator(e)

		case <-motorTimeout.C:
			if e.Behaviour == lib.EB_Moving {
				elevio.SetMotorDirection(e.Direction)
			}
			motorTimeout.Reset(_MOTOR_TIMEOUT * time.Second)

		case <-idleTimeout.C:
			if e.Behaviour == lib.EB_Idle {
				ch_localstateUpdate <- lib.DupElevator(e)
			}
			idleTimeout.Reset(_IDLE_UPDATE_FREQUENCY * time.Second)
		}
	}
}

func isObstructed(e lib.Elevator, timer *time.Timer) bool {
	if e.Behaviour == lib.EB_DoorOpen && !timer.Stop() {
		return true
	}
	return false
}

func onObstruction(e lib.Elevator, timer *time.Timer, obstructed bool) (newelev lib.Elevator, newtimer *time.Timer) {
	switch e.Behaviour {
	case lib.EB_DoorOpen:
		if obstructed {
			timer.Stop()
			select {
			case <-timer.C:
				break
			default:
				break
			}
		} else {
			timer.Reset(time.Duration(e.Cfg.DoorOpenDuration_s) * time.Second)
		}
	default:
		break
	}
	newelev = e
	newtimer = timer
	return
}

func onInitBetweenFloors(e lib.Elevator) lib.Elevator {
	e.Direction = lib.MD_Down
	elevio.SetMotorDirection(e.Direction)
	e.Behaviour = lib.EB_Moving
	return e
}

func onRequestButtonPress(btn_floor int, btn_type lib.ButtonType, timer *time.Timer, e lib.Elevator) (newelev lib.Elevator, newtimer *time.Timer) {
	switch e.Behaviour {
	case lib.EB_DoorOpen:
		if e.Floor == btn_floor {
			if !isObstructed(e, timer) {
				timer.Reset(time.Duration(e.Cfg.DoorOpenDuration_s) * time.Second)
			}
		} else {
			e.Orders[btn_floor][btn_type] = 1
		}
		break
	case lib.EB_Moving:
		e.Orders[btn_floor][btn_type] = 1
		break
	case lib.EB_Idle:
		if e.Floor == btn_floor {
			elevio.SetDoorOpenLamp(true)
			timer.Reset(time.Duration(e.Cfg.DoorOpenDuration_s) * time.Second)
			e.Behaviour = lib.EB_DoorOpen
		} else {
			e.Orders[btn_floor][btn_type] = 1
			e.Direction = op.ChooseDirection(e)
			elevio.SetMotorDirection(e.Direction)
			e.Behaviour = lib.EB_Moving
		}
		break
	}
	newelev = e
	newtimer = timer
	return
}

func onFloorArrival(newFloor int, e lib.Elevator, timer *time.Timer) (newelev lib.Elevator, newtimer *time.Timer) {
	e.Floor = newFloor
	elevio.SetFloorIndicator(e.Floor)

	switch e.Behaviour {
	case lib.EB_Moving:
		if lib.ToBool(op.ShouldStop(e)) {
			elevio.SetMotorDirection(lib.MD_Stop)
			elevio.SetDoorOpenLamp(true)
			e = op.ClearAtCurrentFloor(e)
			timer.Reset(time.Duration(e.Cfg.DoorOpenDuration_s) * time.Second)
			e.Behaviour = lib.EB_DoorOpen
		}
	default:
	}
	newelev = e
	newtimer = timer
	return
}

func onDoorTimeout(e lib.Elevator) (newelev lib.Elevator) {
	switch e.Behaviour {
	case lib.EB_DoorOpen:
		e.Direction = op.ChooseDirection(e)
		elevio.SetDoorOpenLamp(false)
		elevio.SetMotorDirection(e.Direction)

		if e.Direction == lib.MD_Stop {
			e.Behaviour = lib.EB_Idle
		} else {
			e.Behaviour = lib.EB_Moving
		}
		break
	default:
		break
	}
	newelev = e
	return
}
