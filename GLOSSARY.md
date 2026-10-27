# secret_inject

Secret values from configured sources and the workflow-specific selections made
from them.

## Language

**Snapshot**:
The complete collection of ordinary workload values from the selected inputs,
before filtering or renaming. Private bootstrap credentials are not snapshot values.
_Avoid_: shell environment (when referring to the unselected collection)

**Bootstrap credential**:
A private value supplied only to a consuming secret provider, either from an
enrolled named record or a declared earlier-source output.
_Avoid_: workload variable, ambient authentication

**Credential revision**:
The store-wide identity of the private credential context under which a workload
snapshot was resolved.
_Avoid_: provider token hash, configuration version
