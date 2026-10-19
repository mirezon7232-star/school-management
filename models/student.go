package models

type Student struct {
    ID        int    `json:"id"`
    Name      string `json:"name"`
    Roll      int    `json:"roll"`
    Class     string `json:"class"`
    Guardian  string `json:"guardian"`
}
