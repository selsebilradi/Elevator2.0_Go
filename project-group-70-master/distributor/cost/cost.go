package cost

import (
	"time"

	"../../localElevator/op"
	"../../type-library/lib"
)

const _TRAVEL_TIME = 2500
const _DOOR_OPEN_TIME = 3000

func simClearFloors(e_old lib.Elevator) lib.Elevator {
	var e lib.Elevator = e_old
	for btn := 0; btn < lib.NumButtons; btn++ {
		e.Orders[e.Floor][btn] = 0
	}
	return e
}

func Cost(e_old lib.Elevator, button lib.ButtonType, floor int) int {
	var e lib.Elevator = e_old
	e.Orders[floor][button] = 1

	var duration time.Duration = 0
	switch e.Behaviour {
	case lib.EB_Idle:
		e.Direction = op.ChooseDirection(e)
		if e.Direction == lib.MD_Stop {
			return int(duration.Milliseconds())
		}
		break

	case lib.EB_Moving:
		duration += (_TRAVEL_TIME / 2) * time.Millisecond
		e.Floor += int(e.Direction)
		break

	case lib.EB_DoorOpen:
		duration -= (_DOOR_OPEN_TIME / 2) * time.Millisecond
	}

	for {
		if lib.ToBool(op.ShouldStop(e)) {
			e = simClearFloors(e)
			if e.Orders[floor][button] == 0 {
				return int(duration.Milliseconds())
			}
			duration += _DOOR_OPEN_TIME * time.Millisecond
			e.Direction = op.ChooseDirection(e)
		}
		e.Floor += int(e.Direction)
		duration += _TRAVEL_TIME * time.Millisecond
	}
}
