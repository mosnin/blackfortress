package runtime

import (
	"context"
	"net/http"
	"time"
)

type FrameworkPosture struct {
	ID                     string         `json:"id"`
	Name                   string         `json:"name"`
	Controls               int            `json:"controls"`
	ControlsImplemented    int            `json:"controls_implemented"`
	ControlsInProgress     int            `json:"controls_in_progress"`
	ControlsNotStarted     int            `json:"controls_not_started"`
	ControlsWithoutMeasure int            `json:"controls_without_measure"`
	Measures               map[string]int `json:"measures"`
	Score                  float64        `json:"score"`
}

type Posture struct {
	UpdatedAt  time.Time          `json:"updated_at"`
	Frameworks []FrameworkPosture `json:"frameworks"`
	Guardrails GuardrailSummary   `json:"guardrails"`
}

type GuardrailSummary struct {
	Last24h  int `json:"last_24h"`
	Blocked  int `json:"blocked_24h"`
	Asked    int `json:"asked_24h"`
	Recorded int `json:"recorded_24h"`
}

const postureQuery = `
query Posture($id: ID!) {
  node(id: $id) {
    ... on Organization {
      frameworks(first: 100) {
        edges { node {
          id
          name
          controls(first: 1000) {
            edges { node {
              id
              measures(first: 200) { edges { node { state } } }
            } }
          }
        } }
      }
    }
  }
}`

type postureResponse struct {
	Node struct {
		Frameworks struct {
			Edges []struct {
				Node struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					Controls struct {
						Edges []struct {
							Node struct {
								Measures struct {
									Edges []struct {
										Node struct {
											State string `json:"state"`
										} `json:"node"`
									} `json:"edges"`
								} `json:"measures"`
							} `json:"node"`
						} `json:"edges"`
					} `json:"controls"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"frameworks"`
	} `json:"node"`
}

// FetchPosture computes per-framework implementation status from probod.
// A control counts as implemented when it has at least one measure and all
// of its measures are implemented or not applicable.
func FetchPosture(ctx context.Context, baseURL, token, orgID string) ([]FrameworkPosture, error) {
	var resp postureResponse

	client := &http.Client{Timeout: 30 * time.Second}
	if err := doGraphQL(ctx, client, baseURL+"/api/console/v1/graphql", token, postureQuery, map[string]any{"id": orgID}, &resp); err != nil {
		return nil, err
	}

	out := make([]FrameworkPosture, 0, len(resp.Node.Frameworks.Edges))

	for _, fe := range resp.Node.Frameworks.Edges {
		fp := FrameworkPosture{ID: fe.Node.ID, Name: fe.Node.Name, Measures: map[string]int{}}

		for _, ce := range fe.Node.Controls.Edges {
			fp.Controls++

			measures := ce.Node.Measures.Edges
			if len(measures) == 0 {
				fp.ControlsWithoutMeasure++
				fp.ControlsNotStarted++

				continue
			}

			done, started := 0, 0
			for _, me := range measures {
				st := me.Node.State
				fp.Measures[st]++

				switch st {
				case "IMPLEMENTED", "NOT_APPLICABLE":
					done++
					started++
				case "IN_PROGRESS":
					started++
				}
			}

			switch {
			case done == len(measures):
				fp.ControlsImplemented++
			case started > 0:
				fp.ControlsInProgress++
			default:
				fp.ControlsNotStarted++
			}
		}

		if fp.Controls > 0 {
			fp.Score = float64(fp.ControlsImplemented) / float64(fp.Controls)
		}

		out = append(out, fp)
	}

	return out, nil
}
