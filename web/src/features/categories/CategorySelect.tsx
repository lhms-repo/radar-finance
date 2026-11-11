import type { ComponentProps } from "react";
import { NativeSelect } from "@/components/ui/native-select";
import { useCategories } from "@/features/categories/api";

/** Native select of active subcategories grouped by category, for entry forms (RN-18). */
export function CategorySelect({ placeholder = "Escolha a categoria", ...props }: ComponentProps<"select"> & { placeholder?: string }) {
  const categories = useCategories();
  return (
    <NativeSelect {...props}>
      <option value={0}>{placeholder}</option>
      {(categories.data ?? []).map((c) => (
        <optgroup key={c.id} label={c.name}>
          {(c.children ?? []).map((sub) => (
            <option key={sub.id} value={sub.id}>
              {sub.name}
            </option>
          ))}
        </optgroup>
      ))}
    </NativeSelect>
  );
}
