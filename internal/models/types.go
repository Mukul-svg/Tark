package models

import (
	"time"

	"github.com/google/uuid"
)

type Organization struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type Cluster struct {
	ID         uuid.UUID `json:"id" db:"id"`
	OrgID      uuid.UUID `json:"org_id" db:"org_id"`
	Name       string    `json:"name" db:"name"`
	Region     string    `json:"region" db:"region"`
	Status     string    `json:"status" db:"status"`
	Kubeconfig string    `json:"-" db:"kubeconfig"` // Do not expose JSON by default
	PublicIP   string    `json:"public_ip,omitempty" db:"public_ip"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

type Deployment struct {
	ID         uuid.UUID `json:"id" db:"id"`
	ClusterID  uuid.UUID `json:"cluster_id" db:"cluster_id"`
	ModelName  string    `json:"model_name" db:"model_name"`
	Replicas   int       `json:"replicas" db:"replicas"`
	Status     string    `json:"status" db:"status"`
	ServiceURL string    `json:"service_url,omitempty" db:"service_url"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}
