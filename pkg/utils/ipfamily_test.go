package utils

import (
	"os"
	"testing"
)

func TestGetIPFamilyPolicy(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		setEnv   bool
		expected IPFamilyPolicy
	}{
		{
			name:     "Empty environment defaults to SingleStack",
			envValue: "",
			setEnv:   false,
			expected: IPFamilyPolicySingleStack,
		},
		{
			name:     "Explicit SingleStack",
			envValue: "SingleStack",
			setEnv:   true,
			expected: IPFamilyPolicySingleStack,
		},
		{
			name:     "Explicit PreferDualStack",
			envValue: "PreferDualStack",
			setEnv:   true,
			expected: IPFamilyPolicyPreferDualStack,
		},
		{
			name:     "Lowercase preferdualstack matches correctly",
			envValue: "preferdualstack",
			setEnv:   true,
			expected: IPFamilyPolicyPreferDualStack,
		},
		{
			name:     "Uppercase PREFERDUALSTACK matches correctly",
			envValue: "PREFERDUALSTACK",
			setEnv:   true,
			expected: IPFamilyPolicyPreferDualStack,
		},
		{
			name:     "Mixed case sInGlEsTaCk matches correctly",
			envValue: "sInGlEsTaCk",
			setEnv:   true,
			expected: IPFamilyPolicySingleStack,
		},
		{
			name:     "Mixed case PrEfErDuAlStAcK matches correctly",
			envValue: "PrEfErDuAlStAcK",
			setEnv:   true,
			expected: IPFamilyPolicyPreferDualStack,
		},
		{
			name:     "Unsupported RequireDualStack falls back to SingleStack",
			envValue: "RequireDualStack",
			setEnv:   true,
			expected: IPFamilyPolicySingleStack,
		},
		{
			name:     "Invalid value falls back to SingleStack",
			envValue: "InvalidPolicy",
			setEnv:   true,
			expected: IPFamilyPolicySingleStack,
		},
		{
			name:     "Whitespace trimming and mixed-case normalization handled correctly",
			envValue: "  pReFeRdUaLsTaCk  ",
			setEnv:   true,
			expected: IPFamilyPolicyPreferDualStack,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Save the pre-existing environment variable state and value
			originalVal, existed := os.LookupEnv(EnvIPFamilyPolicy)

			// Ensure we always restore the original state when the test completes
			defer func() {
				if existed {
					os.Setenv(EnvIPFamilyPolicy, originalVal)
				} else {
					os.Unsetenv(EnvIPFamilyPolicy)
				}
			}()

			// Set up the test state
			if tc.setEnv {
				os.Setenv(EnvIPFamilyPolicy, tc.envValue)
			} else {
				os.Unsetenv(EnvIPFamilyPolicy)
			}

			// Run the function and assert
			result := GetIPFamilyPolicy()
			if result != tc.expected {
				t.Errorf("GetIPFamilyPolicy() = %v, expected %v", result, tc.expected)
			}
		})
	}
}

func TestIPFamilyPolicyHelpers(t *testing.T) {
	tests := []struct {
		name         string
		policy       IPFamilyPolicy
		expectedV4   bool
		expectedV6   bool
		expectedDual bool
	}{
		{
			name:         "Empty uninitialized policy defaults V4 to true",
			policy:       "",
			expectedV4:   true,
			expectedV6:   false,
			expectedDual: false,
		},
		{
			name:         "SingleStack enables V4 only",
			policy:       IPFamilyPolicySingleStack,
			expectedV4:   true,
			expectedV6:   false,
			expectedDual: false,
		},
		{
			name:         "PreferDualStack enables V4, V6, and DualStack",
			policy:       IPFamilyPolicyPreferDualStack,
			expectedV4:   true,
			expectedV6:   true,
			expectedDual: true,
		},
		{
			name:         "Invalid or unknown policy enables nothing",
			policy:       IPFamilyPolicy("UnknownPolicy"),
			expectedV4:   false,
			expectedV6:   false,
			expectedDual: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.policy.IsV4Enabled(); got != tc.expectedV4 {
				t.Errorf("%q.IsV4Enabled() = %v, expected %v", tc.policy, got, tc.expectedV4)
			}
			if got := tc.policy.IsV6Enabled(); got != tc.expectedV6 {
				t.Errorf("%q.IsV6Enabled() = %v, expected %v", tc.policy, got, tc.expectedV6)
			}
			if got := tc.policy.IsDualStack(); got != tc.expectedDual {
				t.Errorf("%q.IsDualStack() = %v, expected %v", tc.policy, got, tc.expectedDual)
			}
		})
	}
}
