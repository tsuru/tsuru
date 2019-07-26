

Fuzzy
=====

Fuzzy is a very fast spell checker and query suggester written in Golang. 

Motivation:
- Sajari uses very large queries (hundreds of words) but needs to respond sub-second to these queries where possible. Common spell check algorithms are quite slow or very resource intensive.
- The aim was to achieve spell checks in sub 100usec per word (10,000 / second single core) with at least 70% accuracy and multi-language support.
- Currently we see sub 40usec per word and 68% accuracy for a Levenshtein distance of 2 chars on a 2012 macbook pro (english test set comes from Peter Norvig's article, see http://norvig.com/spell-correct.html). 
- A 500 word query can be spell checked in ~0.02 sec / cpu cores, which is good enough for us.

Notes:
- It is currently executed as a single goroutine per lookup, so undoubtedly this could be much faster using multiple cores, but currently the speed is quite good.
- Accuracy is hit slightly because several correct words don't appear at all in the training text (data/big.txt).
- Fuzzy is a "Symmetric Delete Spelling Corrector", which relates to some blogs by Wolf Garbe at Faroo.com (see http://blog.faroo.com/2012/06/07/improved-edit-distance-based-spelling-correction/)

Config:
- Generally no config is required, but you can tweak the model for your application. 
- `"threshold"` is the trigger point when a word becomes popular enough to build lookup keys for it. Setting this to "1" means any instance of a given word makes it a legitimate spelling. This typically corrects the most errors, but can also cause false positives if incorrect spellings exist in the training data. It also causes a much larger index to be built. By default this is set to 4.
- `"depth"` is the Levenshtein distance the model builds lookup keys for. For spelling correction, a setting of "2" is typically very good. At a distance of "3" the potential number of words is much, much larger, but adds little benefit to accuracy. For query prediction a larger number can be useful, but again is much more expensive. **A depth of "1" and threshold of "1" for the 1st Norvig test set gives 63% correction accuracy at ~5usec per check (e.g. ~200kHz)**, for many applications this will be good enough. At depths > 2, the false positives begin to hurt the accuracy.

Future improvements:
- Make some of the expensive processes concurrent. 
- Add spelling checks for different languages. If you have misspellings in different languages please add them or send to us.
- Allow the term-score map to be read from an external term set (e.g. integrating this currently may double up on keeping a term count).
- Currently there is no method to delete lookup keys, so potentially this may cause bloating over time if the dictionary changes signficantly.
- Add right to left deletion beyond Levenshtein config depth (e.g. don't process all deletes accept for query predictors).

Usage:
- Below is some example code showing how to use the package.
- An example showing how to train with a static set of words is contained in the fuzzy_test.go file, which uses the "big.text" file to create an english dictionary. 
- To integrate with your application (e.g. custom dictionary / word popularity), use the single word and multiword training functions shown in the example below. Each time you add a new instance of a given word, pass it to this function. The model will keep a count and 
- We haven't tested with other langauges, but this should work fine. Please let us know how you go? `support@sajari.com`


```go
package main 

import(
	"github.com/sajari/fuzzy"
	"fmt"
)

func main() {
	model := fuzzy.NewModel()

	// For testing only, this is not advisable on production
	model.SetThreshold(1)

	// This expands the distance searched, but costs more resources (memory and time). 
	// For spell checking, "2" is typically enough, for query suggestions this can be higher
	model.SetDepth(5)

	// Train multiple words simultaneously by passing an array of strings to the "Train" function
	words := []string{"bob", "your", "uncle", "dynamite", "delicate", "biggest", "big", "bigger", "aunty", "you're"}
	model.Train(words)
	
	// Train word by word (typically triggered in your application once a given word is popular enough)
	model.TrainWord("single")

	// Check Spelling
	fmt.Println("\nSPELL CHECKS")
	fmt.Println("	Deletion test (yor) : ", model.SpellCheck("yor"))
	fmt.Println("	Swap test (uncel) : ", model.SpellCheck("uncel"))
	fmt.Println("	Replace test (dynemite) : ", model.SpellCheck("dynemite"))
	fmt.Println("	Insert test (dellicate) : ", model.SpellCheck("dellicate"))
	fmt.Println("	Two char test (dellicade) : ", model.SpellCheck("dellicade"))

	// Suggest completions
	fmt.Println("\nQUERY SUGGESTIONS")
	fmt.Println("	\"bigge\". Did you mean?: ", model.Suggestions("bigge", false))
	fmt.Println("	\"bo\". Did you mean?: ", model.Suggestions("bo", false))
	fmt.Println("	\"dyn\". Did you mean?: ", model.Suggestions("dyn", false))

}
```