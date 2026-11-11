import { motion } from "motion/react";
import { navItem } from "@/app/nav";
import { Card } from "@/components/ui/card";

/** Empty screen that tells which phase delivers it. */
export function PlaceholderPage({ id }: { id: string }) {
  const item = navItem(id);
  const Icon = item.icon;
  return (
    <div className="flex flex-col gap-4">
      <h1 className="px-1 text-lg font-semibold md:hidden">{item.label}</h1>
      <Card className="flex flex-col items-center gap-4 px-6 py-14 text-center">
        <motion.span
          initial={{ scale: 0.7, opacity: 0 }}
          animate={{ scale: 1, opacity: 1 }}
          transition={{ type: "spring", stiffness: 400, damping: 22 }}
          className="grid size-16 place-items-center rounded-2xl bg-primary/10"
        >
          <Icon className="size-8 text-primary" />
        </motion.span>
        <div className="flex max-w-md flex-col gap-1">
          <h2 className="text-base font-semibold">{item.label}</h2>
          <p className="text-sm text-muted-foreground">{item.description}</p>
        </div>
        <span className="rounded-full bg-muted px-3 py-1 text-xs font-medium text-muted-foreground">
          Chega na fase {item.phase}
        </span>
      </Card>
    </div>
  );
}
