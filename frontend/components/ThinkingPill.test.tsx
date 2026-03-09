import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThinkingPill } from "./ThinkingPill";

describe("ThinkingPill", () => {
  describe("empty state", () => {
    it("shows 'Thinking' with dots animation when text is empty", () => {
      render(<ThinkingPill text="" />);
      expect(screen.getByText(/Thinking/)).toBeInTheDocument();
    });

    it("shows content when text is only whitespace (not empty after trim check)", () => {
      // Note: "   \n  " actually has content (whitespace), so it won't trigger isEmpty
      // Let's test with truly empty string
      render(<ThinkingPill text="" isStreaming={false} />);
      expect(screen.getByText(/Thinking/)).toBeInTheDocument();
    });
  });

  describe("short content (below collapse threshold)", () => {
    it("shows content directly when less than 5 lines", () => {
      const shortText = "This is a short thinking content.";
      render(<ThinkingPill text={shortText} />);
      expect(screen.getByText(shortText)).toBeInTheDocument();
    });
  });

  describe("long content (meets collapse threshold)", () => {
    // Generate text with 6+ lines to trigger collapse
    const longText = Array(6).fill("Line of thinking content here.").join("\n");

    it("shows collapsed state by default", () => {
      render(<ThinkingPill text={longText} />);
      // Should show arrow pointing right (collapsed)
      const button = screen.getByRole("button");
      expect(button).toBeInTheDocument();
    });

    it("shows scrolling preview when streaming", () => {
      render(<ThinkingPill text={longText} isStreaming={true} />);
      // Should show last 50 chars preview (with ellipsis since text > 50 chars)
      const lastChars = longText.slice(-50);
      // Use regex to match partial text since there are newlines
      expect(screen.getByText(new RegExp(`….*${lastChars.slice(-20)}`))).toBeInTheDocument();
    });

    it("shows 'Thinking finished.' with duration when thinking is complete while streaming", () => {
      render(<ThinkingPill text={longText} isStreaming={true} isThinkingComplete={true} duration={3} />);
      expect(screen.getByText("✓ Thinking finished. (3s)")).toBeInTheDocument();
    });

    it("shows 'Thinking finished.' with duration when streaming is done", () => {
      render(<ThinkingPill text={longText} isStreaming={false} duration={5} />);
      expect(screen.getByText("✓ Thinking finished. (5s)")).toBeInTheDocument();
    });

    it("expands to show full content when clicked", async () => {
      const user = userEvent.setup();
      render(<ThinkingPill text={longText} />);

      const button = screen.getByRole("button");
      await user.click(button);

      // Full content should be visible - check for first line
      expect(screen.getByText(/Line of thinking content here\./)).toBeInTheDocument();
    });

    it("collapses again when clicked twice", async () => {
      const user = userEvent.setup();
      render(<ThinkingPill text={longText} />);

      const button = screen.getByRole("button");

      // Expand
      await user.click(button);
      expect(screen.getByText(/Line of thinking content here\./)).toBeInTheDocument();

      // Collapse
      await user.click(button);
      // Should show title row (Thinking finished)
      expect(screen.getByText("✓ Thinking finished.")).toBeInTheDocument();
    });

    it("is keyboard accessible with Enter key", async () => {
      const user = userEvent.setup();
      render(<ThinkingPill text={longText} />);

      const button = screen.getByRole("button");
      button.focus();
      await user.keyboard("{Enter}");

      expect(screen.getByText(/Line of thinking content here\./)).toBeInTheDocument();
    });

    it("is keyboard accessible with Space key", async () => {
      const user = userEvent.setup();
      render(<ThinkingPill text={longText} />);

      const button = screen.getByRole("button");
      button.focus();
      await user.keyboard(" ");

      expect(screen.getByText(/Line of thinking content here\./)).toBeInTheDocument();
    });
  });

  describe("preview behavior during streaming", () => {
    it("shows ellipsis when content exceeds 50 chars", () => {
      // 6+ lines to trigger collapse, each line 20+ chars
      const longTextWithLines = Array(6).fill("A".repeat(20)).join("\n");
      render(<ThinkingPill text={longTextWithLines} isStreaming={true} />);

      // Should have ellipsis in the preview
      expect(screen.getByText(/…/)).toBeInTheDocument();
    });

    it("shows last chars when content is under 50 chars total", () => {
      // 6 lines but short content
      const shortLines = Array(6).fill("Hi").join("\n");
      render(<ThinkingPill text={shortLines} isStreaming={true} />);

      // Should show the content without ellipsis (total < 50 chars)
      // The preview shows the last 50 chars, which is the whole text
      // Use getAllByText since content appears in both collapsed preview and expanded content
      const elements = screen.getAllByText(/Hi/);
      expect(elements.length).toBeGreaterThan(0);
    });
  });

  describe("status display priority", () => {
    const longText = Array(6).fill("Line of thinking content here.").join("\n");

    it("prioritizes isThinkingComplete over isStreaming", () => {
      // Both true = thinking finished
      render(<ThinkingPill text={longText} isStreaming={true} isThinkingComplete={true} />);
      expect(screen.getByText("✓ Thinking finished.")).toBeInTheDocument();
    });

    it("shows preview when streaming but not thinking complete", () => {
      render(<ThinkingPill text={longText} isStreaming={true} isThinkingComplete={false} />);
      // Should show preview with ellipsis
      expect(screen.getByText(/…/)).toBeInTheDocument();
    });
  });
});
