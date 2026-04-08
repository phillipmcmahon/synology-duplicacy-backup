package main

func badFormat() {
	x := 1
	if x == 1 {
		y := 2
		_ = y
	}
}
