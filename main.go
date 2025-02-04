package main

import (
	"ExploreCaching/quiet_hn/hn"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

func main() {
	// parse flags
	var port, numStories int
	flag.IntVar(&port, "port", 3000, "the port to start the web server on")
	flag.IntVar(&numStories, "num_stories", 30, "the number of top stories to display")
	flag.Parse()

	tpl := template.Must(template.ParseFiles("./index.gohtml"))

	http.HandleFunc("/", handler(numStories, tpl))

	// Start the server
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func handler(numStories int, tpl *template.Template) http.HandlerFunc {

    sc:=switchCache{
        numStories: numStories,
        cacheDureation:  3* time.Second,
    }
   go func(){ //prevent running into slow page load
        ticker:=time.NewTicker(3 * time.Second)
        for {
        
            temp:=switchCache{
                numStories:numStories,
                cacheDureation: 6* time.Second,
            }
            temp.stories()
            sc.cacheMutex.Lock()
            sc.cache=temp.cache
            sc.cacheExpiration=temp.cacheExpiration
            sc.cacheMutex.Unlock()//overwriting data and unlocking, so user should not see refresh delay
             <-ticker.C                   
        }
   }()

    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        stories, err := getCachedstories(sc.numStories) /*numStories =30 */
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
        }
        data := templateData{
            Stories: stories,
            Time:    time.Now().Sub(start),
        }
        err = tpl.Execute(w, data)
        if err != nil {
            http.Error(w, "Failed to process the template", http.StatusInternalServerError)
            return
        }
    })
}



/* Bonus exercise */

type switchCache struct{
    cache []item
    numStories int
    useA bool
    cacheDureation time.Duration
    cacheExpiration time.Time
    cacheMutex sync.Mutex


}

func (sc * switchCache) stories()([]item,error){
    sc.cacheMutex.Lock()
    

    //the web request
    defer sc.cacheMutex.Unlock() //eliminate api alls

    if time.Now().Sub(sc.cacheExpiration)< 0{//current_time.Sub(given_time)
        return sc.cache,nil
            
    }

    stories, err := getTopStories(sc.numStories) /*numStories =30 */
    sc.cacheExpiration=time.Now().Add(sc.cacheDureation)
    if err!=nil{
     return nil, err
    }
    sc.cache=stories
        return sc.cache,nil

    

}



/* Add caching ->increase from seconds->milliseconds->nanoseconds*/

var (
    cache []item
    cacheExperiation time.Time
    cacheMutex sync.Mutex
)

func getCachedstories(numStories int) ([]item, error){
    cacheMutex.Lock()//2nd goroutine will sit/wait until the current gorountine unlocks when updating
    //the web request
    defer cacheMutex.Unlock() //eliminate api alls

    if time.Now().Sub(cacheExperiation)< 0{//current_time.Sub(given_time)
                  return cache,nil
    }
    stories, err := getTopStories(numStories) /*numStories =30 */
    if err!=nil{
     return nil, err
    }
    cache=stories
    cacheExperiation=time.Now().Add(15*time.Second)//whenever cache expires, will have an initial slow load agian
    return cache, nil
}



func getTopStories(numStories int) ([]item, error) {
    var client hn.Client
    ids, err := client.TopItems()
    if err != nil {
        return nil, errors.New("Failed to load top stories")

    }
    /*Retrieve Stories*/
    var stories []item
    at := 0
    for len(stories) < numStories {
        need := numStories - len(stories)
        stories = append(stories, getStories(ids[at:at+need])...)

        at += need //increment through array
        //fmt.Println("the stories", stories)
    }

    return stories, nil
    /**/
}


func getStories(ids []int) []item {
	var stories []item
	//var client hn.Client //add the client inside the gorountine to eliminate race conditions

	type result struct {
		idx  int
		item item
		err  error
	}

	resultCh := make(chan result)
	for i := 0; i < len(ids); i++ {
		go func(idx, id int) {
			var client hn.Client
			hnItem, err := client.GetItem(id)//this is where race condition occurs, one gourotine checking client and another assining 
			if err != nil {
				resultCh <- result{idx: idx, err: err}
			}
			resultCh <- result{idx: idx, item: parseHNItem(hnItem)} //pass into Channel
		}(i, ids[i])
	}

	var results []result
	for i := 0; i < len(ids); i++ {
		results = append(results, <-resultCh)
	}

	sort.Slice(results, func(i, j int) bool {//sort listing
		//fmt.Println(results[i].item.Title, "vs",results[j].item.Title)
		return results[i].idx < results[j].idx
	})

	for _, res := range results {
		if res.err != nil {
			continue
		}
		if isStoryLink(res.item) {
			stories = append(stories, res.item)

		}
	}

	return stories
}



func isStoryLink(item item) bool {
	return item.Type == "story" && item.URL != ""
}

func parseHNItem(hnItem hn.Item) item {
	ret := item{Item: hnItem}
	url, err := url.Parse(ret.URL)
	if err == nil {
		ret.Host = strings.TrimPrefix(url.Hostname(), "www.")
	}
	return ret
}

// item is the same as the hn.Item, but adds the Host field
type item struct {
	hn.Item
	Host string
}

type templateData struct {
	Stories []item
	Time    time.Duration
}
