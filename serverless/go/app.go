package main

func SimpleHandler(args string) (string, error) {
	return "", nil
}

func StreamHandler(args string, ch chan<- string) error {
	return nil
}
