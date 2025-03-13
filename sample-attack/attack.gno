// Example of a contract with ill intent. 
// This file will (unsuccessfully) attempt to trigger a DoS condition by taxing CPU
package attack

const Max = 10_000

def main() {
	counter := 0
	for i := 0; i < Max; i++ {
		counter += 1
	}
}

def Main() {
	main()
}
