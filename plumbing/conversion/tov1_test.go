package conversion_test

import (
	"code.cloudfoundry.org/loggregator/plumbing/conversion"
	v2 "code.cloudfoundry.org/loggregator/plumbing/v2"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Tov1", func() {
	It("doesn't modify the input data", func() {
		tags := make(map[string]string)
		dTags := make(map[string]*v2.Value)

		tags["foo"] = "bar"
		dTags["foo"] = &v2.Value{Data: &v2.Value_Text{Text: "baz"}}

		v2e := &v2.Envelope{
			Message: &v2.Envelope_Log{
				Log: &v2.Log{
					Payload: []byte("hello"),
				},
			},
			Tags:           tags,
			DeprecatedTags: dTags,
		}

		conversion.ToV1(v2e)
		Expect(v2e.Tags["foo"]).To(Equal("bar"))
	})
})
