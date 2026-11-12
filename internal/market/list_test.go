package market

import (
	"errors"
	"testing"
)

// TestLists creates a list, adds products with whole units or kg, merges repeated products, marks picked and edits (RF-71).
func TestLists(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	cafe := f.mustProduct(t, "Café", "Marca Ficticia", 500_000, UnitGram)
	banana, _ := f.s.CreateProduct(ctx, f.userID, ProductInput{Name: "Banana prata", Brand: "Sem marca", ByWeight: true})

	if _, err := f.s.CreateList(ctx, f.userID, "  "); !errors.Is(err, ErrListName) {
		t.Fatalf("empty name: %v", err)
	}
	l, err := f.s.CreateList(ctx, f.userID, "Compra do mês")
	if err != nil {
		t.Fatal(err)
	}
	steps := []struct {
		name      string
		product   int64
		qty       int64
		wantErr   error
		wantItems int
		wantQty   map[int64]int64
	}{
		{name: "two packs of coffee", product: cafe.ID, qty: 2_000, wantItems: 1, wantQty: map[int64]int64{cafe.ID: 2_000}},
		{name: "1,5 kg of banana", product: banana.ID, qty: 1_500, wantItems: 2, wantQty: map[int64]int64{cafe.ID: 2_000, banana.ID: 1_500}},
		{name: "coffee again adds up", product: cafe.ID, qty: 1_000, wantItems: 2, wantQty: map[int64]int64{cafe.ID: 3_000, banana.ID: 1_500}},
		{name: "half a pack is not allowed", product: cafe.ID, qty: 500, wantErr: ErrListQuantity},
		{name: "zero", product: banana.ID, qty: 0, wantErr: ErrListQuantity},
		{name: "unknown product", product: 999, qty: 1_000, wantErr: ErrProductNotFound},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			if err := f.s.AddItem(ctx, l.ID, st.product, st.qty); !errors.Is(err, st.wantErr) {
				t.Fatalf("err = %v, want %v", err, st.wantErr)
			}
			if st.wantErr != nil {
				return
			}
			got, err := f.s.ListDetail(ctx, l.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Items) != st.wantItems {
				t.Fatalf("items = %+v", got.Items)
			}
			for _, it := range got.Items {
				if it.QuantityMilli != st.wantQty[it.Product.ID] {
					t.Fatalf("%s = %d", it.Product.Name, it.QuantityMilli)
				}
			}
		})
	}

	got, _ := f.s.ListDetail(ctx, l.ID)
	bananaItem := got.Items[0]
	if bananaItem.Product.ID != banana.ID {
		t.Fatalf("items come by name: %+v", got.Items)
	}
	if err := f.s.SetPicked(ctx, f.userID, bananaItem.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := f.s.UpdateItem(ctx, got.Items[1].ID, 1_000); err != nil {
		t.Fatal(err)
	}
	got, _ = f.s.ListDetail(ctx, l.ID)
	// Picked items go to the end and keep who picked them.
	if got.Items[1].ID != bananaItem.ID || !got.Items[1].Picked || got.Items[1].PickedByName != "Ana Ficticia" || got.Items[0].QuantityMilli != 1_000 {
		t.Fatalf("after pick = %+v", got.Items)
	}
	lists, _ := f.s.Lists(ctx)
	if len(lists) != 1 || lists[0].ItemCount != 2 || lists[0].PickedCount != 1 || lists[0].Done {
		t.Fatalf("lists = %+v", lists)
	}
	if err := f.s.DeleteProduct(ctx, cafe.ID); !errors.Is(err, ErrProductInUse) {
		t.Fatalf("delete product on a list: %v", err)
	}
	if err := f.s.RemoveItem(ctx, got.Items[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.RemoveItem(ctx, 999); !errors.Is(err, ErrListItemNotFound) {
		t.Fatalf("missing item: %v", err)
	}
	if err := f.s.RenameList(ctx, l.ID, "Compra da semana"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DeleteList(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ListDetail(ctx, l.ID); !errors.Is(err, ErrListNotFound) {
		t.Fatalf("deleted list: %v", err)
	}
}
