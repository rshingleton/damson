package api

import (
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A request that reaches /access/me without a principal is refused, not
// answered with an empty list that reads as "no roles".
func TestMyAccessWithoutAPrincipalIsUnauthorized(t *testing.T) {
	_, err := getMyAccess(t.Context(), deps{})
	require.Error(t, err)
	var se huma.StatusError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, http.StatusUnauthorized, se.GetStatus())
}
