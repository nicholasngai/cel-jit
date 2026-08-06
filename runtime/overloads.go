package runtime

func AddInt64(a, b IntValue) IntValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	return IntValue{Val: a.Val + b.Val}
}
