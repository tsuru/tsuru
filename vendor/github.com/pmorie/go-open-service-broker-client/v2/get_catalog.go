package v2

import (
	"fmt"
	"net/http"
)

func (c *client) GetCatalog() (*CatalogResponse, error) {
	fullURL := fmt.Sprintf(catalogURL, c.URL)

	response, err := c.prepareAndDo(http.MethodGet, fullURL, nil /* params */, nil /* request body */, nil /* originating identity */)
	if err != nil {
		return nil, err
	}

	switch response.StatusCode {
	case http.StatusOK:
		catalogResponse := &CatalogResponse{}
		if err := c.unmarshalResponse(response, catalogResponse); err != nil {
			return nil, HTTPStatusCodeError{StatusCode: response.StatusCode, ResponseError: err}
		}

		if !c.APIVersion.AtLeast(Version2_13()) {
			for ii := range catalogResponse.Services {
				for jj := range catalogResponse.Services[ii].Plans {
					catalogResponse.Services[ii].Plans[jj].Schemas = nil
				}
			}
		} else if !c.EnableAlphaFeatures {
			for ii := range catalogResponse.Services {
				for jj := range catalogResponse.Services[ii].Plans {
					schemas := catalogResponse.Services[ii].Plans[jj].Schemas
					if schemas != nil {
						if schemas.ServiceBinding != nil {
							removeResponseSchema(schemas.ServiceBinding.Create)
						}
					}
				}
			}
		}

		return catalogResponse, nil
	default:
		return nil, c.handleFailureResponse(response)
	}
}

func removeResponseSchema(p *RequestResponseSchema) {
	if p != nil {
		p.Response = nil
	}
}
