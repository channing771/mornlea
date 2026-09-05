import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Hotbar } from "./Hotbar";
afterEach(cleanup);
it("边缘槽位点击与权威选中分离", () => { const select = vi.fn(); const { container } = render(<Hotbar slots={Array.from({ length: 9 }, () => ({ item: 0, count: 0 }))} selectedIndex={0} onSelect={select}/>); fireEvent.click(screen.getByRole("button", { name: "快捷栏 9：空" })); expect(select).toHaveBeenCalledWith(8); expect(container.querySelector('.hud-slot--selected')?.getAttribute('data-index')).toBe('0'); expect(container.querySelectorAll('.hud-slot--neighbor')).toHaveLength(1); });
it("捕获态快捷栏完全不发事件", () => { render(<Hotbar slots={Array.from({ length: 9 }, () => ({ item: 0, count: 0 }))} selectedIndex={8}/>); for (const button of screen.getAllByRole("button")) {
    expect((button as HTMLButtonElement).disabled).toBe(true);
} });
