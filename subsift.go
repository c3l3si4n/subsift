package main

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

var nameservers = []string{
	"1.0.0.1:53",
	"1.1.1.1:53",
	"134.195.4.2:53",
	"149.112.112.112:53",
	"159.89.120.99:53",
	"185.228.168.9:53",
	"185.228.169.9:53",
	"195.46.39.39:53",
	"195.46.39.40:53",
	"205.171.2.65:53",
	"205.171.3.65:53",
	"208.67.220.220:53",
	"208.67.222.222:53",
	"216.146.35.35:53",
	"216.146.36.36:53",
	"64.6.64.6:53",
	"64.6.65.6:53",
	"74.82.42.42:53",
	"76.223.122.150:53",
	"76.76.10.0:53",
	"76.76.19.19:53",
	"76.76.2.0:53",
	"77.88.8.1:53",
	"77.88.8.8:53",
	"8.20.247.20:53",
	"8.26.56.26:53",
	"8.8.4.4:53",
	"8.8.8.8:53",
	"84.200.69.80:53",
	"84.200.70.40:53",
	"89.233.43.71:53",
	"9.9.9.9:53",
	"91.239.100.100:53",
	"94.140.14.14:53",
	"94.140.15.15:53",
}

var index = 0
var indexLock = sync.Mutex{}

func main() {

	log.SetOutput(os.Stderr)

	subdomains := LoadSubdomainsFromFile(os.Args[1])
	ParseSubdomains(subdomains)
}

func GenerateRandLowercaseString() string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz")
	b := make([]rune, 6)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func ResolveDomain(domain string) (bool, error) {
	c := new(dns.Client)
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	// set timeout
	c.Net = "udp"
	c.DialTimeout = 1000 * 1000
	m.RecursionDesired = true
	r, _, err := c.Exchange(m, GetRandomNameserver())
	if err != nil {
		log.Printf("DNS query failed: %s\n", err.Error())
		// check if timeout
		if strings.Contains(err.Error(), "i/o timeout") {
			log.Default().Println("retrying scan for " + domain)
			return ResolveDomain(domain)
		}

		return false, nil
	}
	if r.Rcode != dns.RcodeSuccess {
		log.Default().Println("returned false due to no Success", r.Rcode)
		return false, nil
	}
	if r.Rcode != dns.RcodeNameError {
		log.Default().Println("retrying scan for " + domain)
		return ResolveDomain(domain)
	}
	if len(r.Answer) < 1 {
		log.Default().Println("returned false due to no answers")
		return false, nil
	}
	log.Default().Println("resolved " + domain)
	return true, nil
}

func ResolveDomainRetry(domain string, retries int) bool {
	for i := 0; i < retries; i++ {
		resolved, err := ResolveDomain(domain)
		if err != nil {
			continue
		}
		return resolved
	}

	return false
}

func TestDomainForWildcards(domain string, wildcardMap map[string]bool, mu *sync.Mutex) bool {
	subdomainToTest := GenerateRandLowercaseString() + "." + domain
	if found := ResolveDomainRetry(subdomainToTest, 3); found {
		log.Default().Printf("TestDomainForWildcards %s wildcard.", domain)
		return true
	} else {
		log.Default().Printf("TestDomainForWildcards %s  !wildcard.", domain)

		return false
	}
}

func LoadSubdomainsFromFile(filename string) []string {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	var subdomains []string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		subdomains = append(subdomains, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	return subdomains
}

func GetRandomNameserver() string {
	/*
			185.228.168.9
		149.112.112.112
		1.0.0.1
		1.1.1.1
		216.146.35.35
		208.67.220.220
		208.67.222.222
		216.146.36.36
		64.6.64.6
		64.6.65.6
		205.171.3.65
		195.46.39.40
		76.76.10.0
		76.76.2.0
		8.8.4.4
		195.46.39.39
		185.228.169.9
		134.195.4.2
		8.20.247.20
		9.9.9.9
		205.171.2.65
		8.26.56.26
		94.140.14.14
		77.88.8.1
		94.140.15.15
		84.200.70.40
		76.223.122.150
		76.76.19.19
	*/
	nameserver := nameservers[index%len(nameservers)]
	indexLock.Lock()
	index++
	indexLock.Unlock()
	return nameserver

}

func ParseSubdomains(subdomains []string) []string {
	var parsedSubdomains []string
	var mu sync.Mutex
	// hashmap to track subdomain zones that have already been seen, true if wildcard detected
	wildcardMap := make(map[string]bool)
	wg := &sync.WaitGroup{}
	subdomainChan := make(chan string)
	// worker pool
	numWorkers := 100
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for subdomain := range subdomainChan {
				zone := subdomain[strings.Index(subdomain, ".")+1:]
				mu.Lock()
				val, ok := wildcardMap[zone]
				mu.Unlock()

				if ok && val {
					continue
				}

				if !ok {
					log.Default().Println("Checking for zone: " + zone)

					isWildcardZone := TestDomainForWildcards(zone, wildcardMap, &mu)
					mu.Lock()
					wildcardMap[zone] = isWildcardZone
					mu.Unlock()

					if isWildcardZone {
						continue
					}
				}

				isWildcardSubdomain := TestDomainForWildcards(subdomain, wildcardMap, &mu)
				if !isWildcardSubdomain {
					mu.Lock()
					log.Default().Println("Adding ", subdomain)
					parsedSubdomains = append(parsedSubdomains, subdomain)
					mu.Unlock()
				}
			}
		}()
	}

	// feed the worker pool
	for _, subdomain := range subdomains {
		subdomainChan <- subdomain
	}

	close(subdomainChan)
	wg.Wait()

	for _, subdomain := range parsedSubdomains {
		fmt.Println(subdomain)
	}
	return parsedSubdomains
}
