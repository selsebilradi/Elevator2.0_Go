package op

import (
	"../../type-library/lib"
	"../driver-go/elevio"
)

func toInt(b bool) int {
	var a int = 0
	if b {
		a = 1
	}
	return a
}

func Above(e lib.Elevator) int {
	for f := e.Floor + 1; f < elevio.NumFloors(); f++ {
		for btn := 0; btn < lib.NumButtons; btn++ {
			if lib.ToBool(e.Orders[f][btn]) {
				return 1
			}
		}
	}
	return 0
}

func Below(e lib.Elevator) int {
	for f := 0; f < e.Floor; f++ {
		for btn := 0; btn < lib.NumButtons; btn++ {
			if lib.ToBool(e.Orders[f][btn]) {
				return 1
			}
		}
	}
	return 0
}

func ChooseDirection(e lib.Elevator) lib.MotorDirection {
	switch e.Direction {
	case lib.MD_Up:
		if lib.ToBool(Above(e)) {
			return lib.MD_Up
		} else if lib.ToBool(Below(e)) {
			return lib.MD_Down
		}
		return lib.MD_Stop

	case lib.MD_Down:
		if lib.ToBool(Below(e)) {
			return lib.MD_Down
		} else if lib.ToBool(Above(e)) {
			return lib.MD_Up
		}
		return lib.MD_Stop

	case lib.MD_Stop:
		if lib.ToBool(Below(e)) {
			return lib.MD_Down
		} else if lib.ToBool(Above(e)) {
			return lib.MD_Up
		}
		return lib.MD_Stop

	default:
		return lib.MD_Stop

	}
}

func ShouldStop(e lib.Elevator) int {
	if e.Floor == 0 || e.Floor == elevio.NumFloors()-1 {
		return 1
	}
	switch e.Direction {
	case lib.MD_Down:
		return toInt(lib.ToBool(e.Orders[e.Floor][lib.BT_HallDown]) || lib.ToBool(e.Orders[e.Floor][lib.BT_Cab]) || !lib.ToBool(Below(e)))
	case lib.MD_Up:
		return toInt(lib.ToBool(e.Orders[e.Floor][lib.BT_HallUp]) || lib.ToBool(e.Orders[e.Floor][lib.BT_Cab]) || !lib.ToBool(Above(e)))
	case lib.MD_Stop:
		return 1
	default:
		return 1
	}
}

func ClearAtCurrentFloor(e lib.Elevator) lib.Elevator {
	switch e.Cfg.ClearRequestVariant {
	case lib.CV_All:
		for btn := 0; btn < lib.NumButtons; btn++ {
			e.Orders[e.Floor][btn] = 0
		}
		break

	case lib.CV_InDirection:
		e.Orders[e.Floor][lib.BT_Cab] = 0
		switch e.Direction {
		case lib.MD_Up:
			e.Orders[e.Floor][lib.BT_HallUp] = 0
			if !lib.ToBool(Above(e)) {
				e.Orders[e.Floor][lib.BT_HallDown] = 0
			}
			break

		case lib.MD_Down:
			e.Orders[e.Floor][lib.BT_HallDown] = 0
			if !lib.ToBool(Below(e)) {
				e.Orders[e.Floor][lib.BT_HallUp] = 0
			}
			break

		case lib.MD_Stop:
		default:
			e.Orders[e.Floor][lib.BT_HallUp] = 0
			e.Orders[e.Floor][lib.BT_HallDown] = 0
			break
		}
		break

	default:
		break
	}
	return e
}
