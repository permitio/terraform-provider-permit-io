package roles

func RemoveDoubleQuotes(str string) string {
	return str[1 : len(str)-1]
}
