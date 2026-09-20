package model

import (
	"errors"
	"math"
	"regexp"
	"strings"

	"swatch/internal/models"
)

// Classifier is the TF-IDF + multinomial logistic regression model trained by
// ml/train_review_classifier.py. It ports scikit-learn's TfidfVectorizer
// transform so predictions match the Python model exactly.
type Classifier struct {
	ArtifactInfo
	Params struct {
		Lowercase   bool   `json:"lowercase"`
		NgramMax    int    `json:"ngram_max"`
		SublinearTF bool   `json:"sublinear_tf"`
		Norm        string `json:"norm"`
	} `json:"params"`

	Classes    []models.Ranking `json:"classes"`
	Vocabulary map[string]int   `json:"vocabulary"`
	IDF        []float64        `json:"idf"`
	Coef       [][]float64      `json:"coef"`
	Intercept  []float64        `json:"intercept"`

	ParityFixtures []ParityFixture `json:"parity_fixtures"`
}

// ParityFixture is a text plus the probabilities scikit-learn produced for it.
type ParityFixture struct {
	Text          string    `json:"text"`
	Probabilities []float64 `json:"probabilities"`
}

// Prediction is the classifier's verdict on a review.
type Prediction struct {
	Ranking       models.Ranking     `json:"ranking"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

// Equivalent to scikit-learn's default token_pattern (?u)\b\w\w+\b.
var tokenPattern = regexp.MustCompile(`[\p{L}\p{N}_]{2,}`)

func (c *Classifier) prepare() error {
	if len(c.Vocabulary) == 0 || len(c.Coef) == 0 {
		return errors.New("artifact has no vocabulary or coefficients")
	}
	if len(c.Coef) != len(c.Classes) || len(c.Intercept) != len(c.Classes) {
		return errors.New("coefficients do not match the class count")
	}
	if len(c.IDF) != len(c.Vocabulary) {
		return errors.New("idf vector does not match the vocabulary size")
	}
	if c.Params.NgramMax < 1 {
		c.Params.NgramMax = 1
	}
	return nil
}

// Classify scores a review and returns the predicted ranking with its confidence.
func (c *Classifier) Classify(text string) (Prediction, error) {
	features := c.transform(text)
	if len(features) == 0 {
		return Prediction{}, errors.New("review has no recognisable words")
	}

	logits := make([]float64, len(c.Classes))
	for class := range c.Classes {
		sum := c.Intercept[class]
		for idx, value := range features {
			sum += value * c.Coef[class][idx]
		}
		logits[class] = sum
	}

	probabilities := softmax(logits)
	best := 0
	for i, p := range probabilities {
		if p > probabilities[best] {
			best = i
		}
	}

	byName := make(map[string]float64, len(c.Classes))
	for i, class := range c.Classes {
		byName[class.RankingName] = round4(probabilities[i])
	}
	return Prediction{
		Ranking:       c.Classes[best],
		Confidence:    round4(probabilities[best]),
		Probabilities: byName,
	}, nil
}

// transform reproduces TfidfVectorizer: tokenise, count n-grams in the
// vocabulary, apply sublinear TF and IDF, then L2-normalise.
func (c *Classifier) transform(text string) map[int]float64 {
	if c.Params.Lowercase {
		text = strings.ToLower(text)
	}
	tokens := tokenPattern.FindAllString(text, -1)

	counts := make(map[int]float64)
	for n := 1; n <= c.Params.NgramMax; n++ {
		for i := 0; i+n <= len(tokens); i++ {
			term := strings.Join(tokens[i:i+n], " ")
			if idx, ok := c.Vocabulary[term]; ok {
				counts[idx]++
			}
		}
	}
	if len(counts) == 0 {
		return nil
	}

	features := make(map[int]float64, len(counts))
	var sumSquares float64
	for idx, count := range counts {
		tf := count
		if c.Params.SublinearTF {
			tf = 1 + math.Log(count)
		}
		value := tf * c.IDF[idx]
		features[idx] = value
		sumSquares += value * value
	}
	if c.Params.Norm == "l2" && sumSquares > 0 {
		length := math.Sqrt(sumSquares)
		for idx := range features {
			features[idx] /= length
		}
	}
	return features
}

func softmax(logits []float64) []float64 {
	peak := logits[0]
	for _, v := range logits {
		peak = math.Max(peak, v)
	}
	sum := 0.0
	out := make([]float64, len(logits))
	for i, v := range logits {
		out[i] = math.Exp(v - peak)
		sum += out[i]
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

func round4(f float64) float64 {
	return math.Round(f*10000) / 10000
}
