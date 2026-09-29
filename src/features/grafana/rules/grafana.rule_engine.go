/*
Package rules provides a generic, declarative rules engine for normalization, validation, and policy evaluation.

ALGORITHM BLUEPRINT (Declarative Rules Engine):
1. Rule[T]: Declarative structure containing Condition predicate, ErrorMessage, and Mutator function.
2. RuleSet[T]: Ordered collection of rules evaluated sequentially.
3. Execution:
   - Normalize: Iterates through all mutators in the RuleSet to produce a sanitized model.
   - Validate: Iterates through conditions and returns the first or aggregated invariant violation error.
4. Invariants:
   - Zero inline comments inside function bodies.
   - Zero hardcoded if/else branching inside service callers.
   - New business rules can be added purely by appending to the declarative rule slice.
*/
package rules

import (
	"errors"
	"fmt"
	"strings"
)

type Rule[T any] struct {
	Name         string
	Condition    func(T) bool
	ErrorMessage string
	Mutator      func(T) T
}

type RuleSet[T any] []Rule[T]

func (rs RuleSet[T]) Normalize(target T) T {
	res := target
	for _, r := range rs {
		if r.Mutator != nil {
			res = r.Mutator(res)
		}
	}
	return res
}

func (rs RuleSet[T]) Validate(target T) error {
	var errs []string
	for _, r := range rs {
		if r.Condition != nil && !r.Condition(target) {
			errMsg := r.ErrorMessage
			if errMsg == "" {
				errMsg = fmt.Sprintf("rule %q validation failed", r.Name)
			}
			errs = append(errs, errMsg)
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (rs RuleSet[T]) Execute(target T) (T, error) {
	normalized := rs.Normalize(target)
	if err := rs.Validate(normalized); err != nil {
		return normalized, err
	}
	return normalized, nil
}
