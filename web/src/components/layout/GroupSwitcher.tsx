import { User, Users } from "lucide-react";
import { useId } from "react";
import { useMe, useSwitchGroup } from "@/app/session";
import { cn } from "@/lib/utils";

/** Shows the active group and, for someone in more than one, switches it without entering again (RF-81). */
export function GroupSwitcher({ className }: { className?: string }) {
  const me = useMe();
  const switchGroup = useSwitchGroup();
  const id = useId();
  if (!me) return null;
  // The personal space shows a single person; a group shows many (RF-83).
  const Icon = me.group.personal ? User : Users;
  if (me.groups.length < 2) {
    return (
      <p className={cn("flex items-center gap-2 truncate text-sm text-muted-foreground", className)}>
        <Icon className="size-4 shrink-0" />
        <span className="truncate">{me.group.name}</span>
      </p>
    );
  }
  return (
    <div className={cn("flex items-center gap-2", className)}>
      <label htmlFor={id} className="sr-only">
        Grupo
      </label>
      <Icon className="size-4 shrink-0 text-muted-foreground" />
      <select
        id={id}
        value={me.group.id}
        disabled={switchGroup.isPending}
        onChange={(e) => switchGroup.mutate(Number(e.target.value))}
        className="h-9 min-w-0 flex-1 truncate rounded-lg border border-input bg-card px-2 text-base md:text-sm"
      >
        {me.groups.map((g) => (
          <option key={g.id} value={g.id}>
            {g.name}
          </option>
        ))}
      </select>
    </div>
  );
}
