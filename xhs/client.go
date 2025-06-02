package xhs

import (
	"fmt"
	"net/http"

	"github.com/go-viper/mapstructure/v2"
	"github.com/playwright-community/playwright-go"
	"resty.dev/v3"
)

type Response struct {
	Code    int                    `json:"code"`
	Success bool                   `json:"success"`
	Msg     string                 `json:"msg"`
	Data    map[string]interface{} `json:"data"`
}

func (r *Response) IsSuccess() bool {
	return r.Success && r.Code == 0
}

func GetResponseData[T any](r *Response) (*T, error) {
	var result T
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "json",
		Result:  &result,
	})
	if err != nil {
		return nil, err
	}
	err = decoder.Decode(r.Data)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

type Client struct {
	client     *resty.Client
	userID     string
	xsecToken  string
	xsecSource string
}

func newClient(page playwright.Page, cookies []*http.Cookie) *Client {
	client := resty.New()
	client.SetCookies(cookies)
	client.SetBaseURL("https://edith.xiaohongshu.com")
	client.SetHeaders(map[string]string{
		"User-Agent":   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36",
		"Content-Type": "application/json;charset=UTF-8",
	})
	client.AddRequestMiddleware(func(client *resty.Client, request *resty.Request) error {
		var rawURL string
		var data string
		method := request.Method

		switch method {
		case http.MethodPost:
			rawURL = request.URL
			data = fmt.Sprintf("%s", request.Body)
		case http.MethodGet:
			rawURL = request.URL + "?" + request.QueryParams.Encode()
		default:
			return fmt.Errorf("unknown request method: %s, url: %s", method, request.URL)
		}

		signed, err := sign(page, rawURL, data)
		if err != nil {
			return err
		}

		request.SetHeader("X-s", signed.XS)
		request.SetHeader("X-t", fmt.Sprintf("%d", signed.XT))

		return nil
	})
	return &Client{
		client: client,
	}
}

func (c *Client) Close() error {
	return c.client.Close()
}

type MeInfoResponse struct {
	Guest    bool   `json:"guest"`
	RedID    string `json:"red_id"`
	UserID   string `json:"user_id"`
	Nickname string `json:"nickname"`
	Desc     string `json:"desc"`
	Gender   int    `json:"gender"`
	Images   string `json:"images"`
	Imageb   string `json:"imageb"`
}

func (c *Client) GetMyInfo() (*MeInfoResponse, error) {
	rsp, err := c.client.R().SetResult(&Response{}).
		Get("/api/sns/web/v2/user/me")
	if err != nil {
		return nil, err
	}
	response := rsp.Result().(*Response)
	if !response.IsSuccess() {
		return nil, newError(response.Code, response.Msg)
	}
	return GetResponseData[MeInfoResponse](response)
}

func (c *Client) SetUserID(id string) {
	c.userID = id
}

func (c *Client) SetXsecToken(token string, source string) {
	c.xsecToken = token
	if source == "" {
		source = "pc_feed"
	}
	c.xsecSource = source
}

func (c *Client) GetLikes(cursor string) (*Response, error) {
	rsp, err := c.client.R().
		SetResult(&Response{}).
		SetQueryParams(map[string]string{
			"num":           "30",
			"cursor":        cursor,
			"user_id":       c.userID,
			"image_formats": "jpg,webp,avif",
			"xsec_token":    c.xsecToken,
			"xsec_source":   c.xsecSource,
		}).
		Get("/api/sns/web/v1/note/like/page")
	if err != nil {
		return nil, err
	}
	response := rsp.Result().(*Response)
	if !response.IsSuccess() {
		return nil, newError(response.Code, response.Msg)
	}
	return response, nil
}
