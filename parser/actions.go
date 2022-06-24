package parser

import (
	"fmt"
	"strings"

	goyaml "github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"gopkg.in/yaml.v3"

	"github.com/sethvargo/ratchet/resolver"
)

var _ = goyaml.Unmarshal

type Actions struct{}

// Parse pulls the GitHub Actions refs from the document.
func (a *Actions) Parse(m *ast.File) (*RefsList, error) {
	var refs RefsList

	if m == nil {
		return nil, nil
	}

	for _, c1 := range m.Docs {
		fmt.Printf("c1.Value: %s\n", c1.String())
		switch n := c1.Body.(type) {
		case *ast.CommentNode:
			if n.Comment == nil {
				continue
			}
			for _, c := range n.Comment.Comments {
				fmt.Printf("c: %#v\n", c.Comment.String())
			}
		}
	}

	for _, docMap := range m.Docs {
		if docMap.Type() != ast.DocumentType {
			return nil, fmt.Errorf("expected document node, got %v", docMap.Type())
		}

		// Top-level object map
		switch docMap.Type() {
		case ast.MappingType:
			// nothing to do, go through.
		default:
			if c := strings.Count(docMap.String(), "\n"); c > 0 {
				refs.Add(strings.Repeat("\n", c), docMap)
			}
			continue
		}

		dm := docMap.Body.(*ast.MappingNode)
		for i, topLevelMap := range dm.Values {
			// runs: keyword
			if topLevelMap.String() == "runs" {
				runs := dm.Values[i+1]
				if runs.Type() != ast.MappingType {
					continue
				}

				// Only look at composite actions.
				foundComposite := false
				for j, runMap := range runs {
					if runMap.Value == "using" && len(runs.Content) > j+1 && runs.Content[j+1].Value == "composite" {
						foundComposite = true
						break
					}
				}
				if !foundComposite {
					continue
				}

				// List of steps, iterate over each step and find the "uses" clause.
				for j, runMap := range runs.Content {
					if runMap.Value == "steps" {
						steps := runs.Content[j+1]
						for _, step := range steps.Content {
							if step.Kind != yaml.MappingNode {
								continue
							}

							for k, property := range step.Content {
								if property.Value == "uses" {
									uses := step.Content[k+1]
									// Only include references to remote workflows. This could be
									// a local workflow, which should not be pinned.
									switch {
									case strings.HasPrefix(uses.Value, "docker://"):
										ref := resolver.NormalizeContainerRef(uses.Value)
										refs.Add(ref, uses)
									case strings.Contains(uses.Value, "@"):
										ref := resolver.NormalizeActionsRef(uses.Value)
										refs.Add(ref, uses)
									}
								}
							}
						}
					}
				}
			}

			// jobs: keyword
			if topLevelMap.Value == "jobs" {
				jobs := docMap.Content[i+1]
				if jobs.Kind != yaml.MappingNode {
					continue
				}

				for _, jobMap := range jobs.Content {
					if jobMap.Kind != yaml.MappingNode {
						continue
					}

					for j, sub := range jobMap.Content {
						// Container reference for running the job, should be resolved as a
						// Docker reference.
						if sub.Value == "container" {
							containerMap := jobMap.Content[j+1]
							for k, property := range containerMap.Content {
								if property.Value == "image" {
									image := containerMap.Content[k+1]
									ref := resolver.NormalizeContainerRef(image.Value)
									refs.Add(ref, image)
									break
								}
							}
						}

						// CI service container, should be resolved as a Docker reference.
						// This is a map, so the container value is nested a bit deeper.
						if sub.Value == "services" {
							servicesMap := jobMap.Content[j+1]
							for _, subMap := range servicesMap.Content {
								if subMap.Kind != yaml.MappingNode {
									continue
								}

								for k, property := range subMap.Content {
									if property.Value == "image" {
										image := subMap.Content[k+1]
										ref := resolver.NormalizeContainerRef(image.Value)
										refs.Add(ref, image)
										break
									}
								}
							}
						}

						// List of steps, iterate over each step and find the "uses" clause.
						if sub.Value == "steps" {
							steps := jobMap.Content[j+1]
							for _, step := range steps.Content {
								if step.Kind != yaml.MappingNode {
									continue
								}

								for k, property := range step.Content {
									if property.Value == "uses" {
										uses := step.Content[k+1]
										// Only include references to remote workflows. This could be
										// a local workflow, which should not be pinned.
										switch {
										case strings.HasPrefix(uses.Value, "docker://"):
											ref := resolver.NormalizeContainerRef(uses.Value)
											refs.Add(ref, uses)
										case strings.Contains(uses.Value, "@"):
											ref := resolver.NormalizeActionsRef(uses.Value)
											refs.Add(ref, uses)
										}
									}
								}
							}
						}

						// Top-level uses, likely for a reusable workflow.
						if sub.Value == "uses" {
							uses := jobMap.Content[j+1]

							// Only include references to remote workflows. This could be a
							// local workflow, which should not be pinned.
							switch {
							case strings.HasPrefix(uses.Value, "docker://"):
								ref := resolver.NormalizeContainerRef(uses.Value)
								refs.Add(ref, uses)
							case strings.Contains(uses.Value, "@"):
								ref := resolver.NormalizeActionsRef(uses.Value)
								refs.Add(ref, uses)
							}
						}
					}
				}
			}
		}
	}

	return &refs, nil
}
