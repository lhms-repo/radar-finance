import { DropdownMenu } from "radix-ui";
import type { ReactNode } from "react";
import { useBackLayer } from "@/lib/back";

/** Dropdown menu root that the back button closes on the phone (RNF-26). */
export function MenuRoot({ children }: { children: ReactNode }) {
  const menu = useBackLayer();
  return (
    <DropdownMenu.Root open={menu.open} onOpenChange={menu.setOpen}>
      {children}
    </DropdownMenu.Root>
  );
}
