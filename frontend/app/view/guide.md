# Building Views in Wave Terminal

## Overview

Wave Terminal's view system provides a flexible architecture for creating both simple and complex terminal interfaces. Each view consists of two main parts:

- A **ViewModel** that manages state and behavior
- A **View Component** that handles rendering

## Core Concepts

### ViewModel

Every view implements the `ViewModel` interface which defines the basic contract:

```typescript
interface ViewModel {
  // Required
  viewType: string; // Unique identifier for the view type

  // Optional UI Elements
  viewIcon?: jotai.Atom<string | IconButtonDecl>; // Icon in header
  viewName?: jotai.Atom<string>; // Display name
  viewText?: jotai.Atom<string | HeaderElem[]>; // Header content
  preIconButton?: jotai.Atom<IconButtonDecl>; // Left header button
  endIconButtons?: jotai.Atom<IconButtonDecl[]>; // Right header buttons

  // View Behavior
  blockBg?: jotai.Atom<MetaType>; // Background styling
  manageConnection?: jotai.Atom<boolean>; // Connection handling
  noPadding?: jotai.Atom<boolean>; // Layout control

  // Optional Methods
  onBack?: () => void;
  onForward?: () => void;
  onSearchChange?: (text: string) => void;
  onSearch?: (text: string) => void;
  getSettingsMenuItems?: () => ContextMenuItem[];
  giveFocus?: () => boolean;
  keyDownHandler?: (e: WaveKeyboardEvent) => boolean;
  dispose?: () => void;
}
```

### State Management

Views use Jotai for atomic state management. Each property that needs to be reactive should be declared as an atom:

```typescript
class MyViewModel implements ViewModel {
  viewType = "myview";

  // Simple static atom
  viewIcon = atom("sparkles");

  // Derived atom based on block metadata
  viewName = atom((get) => {
    const blockData = get(this.blockAtom);
    return blockData?.meta?.["myview:name"] ?? "My View";
  });
}
```

### Block Integration

Each view is associated with a block in Wave Terminal:

```typescript
class MyViewModel implements ViewModel {
  blockId: string;
  blockAtom: jotai.Atom<Block>;

  constructor(blockId: string) {
    this.blockId = blockId;
    this.blockAtom = WOS.getWaveObjectAtom<Block>(`block:${blockId}`);
  }
}
```

### Header Elements

Views can customize their header using various element types:

```typescript
type HeaderElem =
  | IconButtonDecl // Icon buttons
  | HeaderText // Static text
  | HeaderInput // Text input
  | HeaderDiv // Container
  | HeaderTextButton // Text buttons
  | ConnectionButton // Connection status
  | MenuButton; // Dropdown menu
```

Example usage:

```typescript
this.viewText = atom((get) => {
  return [
    {
      elemtype: "iconbutton",
      icon: "refresh",
      title: "Reload",
      click: () => this.reload(),
    },
    {
      elemtype: "text",
      text: "Status: Ready",
    },
  ];
});
```

### View Component

The React component that renders your view:

```typescript
interface MyViewProps {
    blockId: string;
    model: MyViewModel;
}

const MyView = ({ blockId, model }: MyViewProps) => {
    return (
        <div className="my-view">
            {/* View content */}
        </div>
    );
};
```

### Metadata

Views can store and access persistent configuration through block metadata:

```typescript
// Reading metadata
const setting = atom((get) => {
  const blockData = get(this.blockAtom);
  return blockData?.meta?.["myview:setting"] ?? defaultValue;
});

// Writing metadata
RpcApi.SetMetaCommand(TabRpcClient, {
  oref: WOS.makeORef("block", this.blockId),
  meta: { "myview:setting": newValue },
});
```

## Creating a New View

### 1. Define Your ViewModel

```typescript
class MyViewModel implements ViewModel {
  viewType = "myview";
  blockId: string;
  blockAtom: jotai.Atom<Block>;

  // Custom state
  dataAtom: jotai.Atom<MyDataType>;

  constructor(blockId: string) {
    this.blockId = blockId;
    this.blockAtom = WOS.getWaveObjectAtom<Block>(`block:${blockId}`);

    // Initialize your atoms
    this.dataAtom = atom(initialData);
  }

  // Add methods for your view's behavior
  reload() {
    // Implementation
  }

  dispose() {
    // Cleanup
  }
}
```

### 2. Create a Factory Function

```typescript
function makeMyViewModel(blockId: string): MyViewModel {
  return new MyViewModel(blockId);
}
```

### 3. Implement the View Component

```typescript
const MyView = ({ blockId, model }: { blockId: string; model: MyViewModel }) => {
    // Use hooks to access state
    const data = useAtomValue(model.dataAtom);

    return (
        <div className="my-view">
            {/* Render your view */}
        </div>
    );
};
```

## Best Practices

### State Management

- Use atoms for any state that needs to be reactive
- Derive computed values using atom getters
- Keep complex state transformations in the model
- Use block metadata for persistent settings

### UI Design

- Follow Wave Terminal's UI patterns
- Use header elements for primary actions
- Implement context menu for settings
- Handle keyboard shortcuts via keyDownHandler

### Cleanup

- Clean up resources in dispose()
- Remove event listeners
- Clear intervals/timeouts
- Unregister from global systems

### Performance

- Memoize expensive computations
- Use React.memo for pure components
- Avoid creating atoms in render functions
- Cache atom references when possible

## Examples

### Simple Static View

```typescript
// Model
class StaticViewModel implements ViewModel {
    viewType = "static";
    viewIcon = atom("info");
    viewName = atom("Info View");

    constructor(blockId: string) {
        // Basic initialization
    }
}

// View
const StaticView = ({ model }: { model: StaticViewModel }) => {
    return <div>Static Content</div>;
};
```

### Interactive View

```typescript
// Model
class InteractiveViewModel implements ViewModel {
    viewType = "interactive";
    countAtom = atom(0);

    viewText = atom((get) => [{
        elemtype: "iconbutton",
        icon: "plus",
        click: () => this.increment()
    }]);

    increment() {
        globalStore.set(this.countAtom, (n) => n + 1);
    }
}

// View
const InteractiveView = ({ model }: { model: InteractiveViewModel }) => {
    const count = useAtomValue(model.countAtom);
    return <div>Count: {count}</div>;
};
```

## Advanced Topics

### VDOM Integration

Views can integrate with Wave's VDOM system to provide rich UI capabilities:

```typescript
class HybridViewModel implements ViewModel {
  termMode = atom((get) => {
    const blockData = get(this.blockAtom);
    return blockData?.meta?.["term:mode"] ?? "term";
  });

  viewIcon = atom((get) => {
    const mode = get(this.termMode);
    return mode === "vdom" ? "bolt" : "terminal";
  });
}
```

### Custom Settings

Views can provide custom settings through the context menu:

```typescript
getSettingsMenuItems(): ContextMenuItem[] {
    return [
        {
            label: "View Settings",
            submenu: [
                {
                    label: "Option 1",
                    type: "checkbox",
                    checked: true,
                    click: () => this.toggleOption()
                }
            ]
        }
    ];
}
```
