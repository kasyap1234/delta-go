# Product Guidelines

## Documentation & Communication
- **Minimalist Approach:** Prioritize clear, self-documenting code through descriptive naming conventions. 
- **Code Comments:** Use sparingly, focusing only on complex logic or critical "why" decisions that aren't immediately obvious from the code structure.

## Design Philosophy
- **Robustness over Complexity:** Favor simple, reliable, and well-tested components. In high-stakes trading, simplicity is a feature that aids in debugging and system stability.
- **Fail-Fast & Recovery:** Design systems to identify errors quickly and attempt graceful recovery where safe.

## User Interface (CLI & Logs)
- **Visual Clarity:** Utilize formatting and color-coding in CLI output to provide immediate visual cues for critical system events such as trade executions, risk limit hits, and system errors.
- **Actionable Logging:** Ensure logs provide enough context to understand system state during significant events without creating excessive noise.

## Error Handling & Resilience
- **Resilient Execution:** Implement robust retry logic with exponential backoff for non-fatal errors like API timeouts or transient network issues.
- **Safety First:** Maintain system integrity by ensuring that unrecoverable errors are logged with full context for post-mortem analysis.

## Quality Assurance & Testing
- **Verification-Heavy:** Maintain rigorous unit test coverage for all core logic, with a specific focus on risk management, order parsing, and strategy calculations.
- **Integration Validation:** Prioritize end-to-end integration tests that simulate the full trading lifecycle from data ingestion to order execution in a sandbox environment to ensure component harmony.
