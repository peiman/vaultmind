package cmd

import "github.com/spf13/cobra"

var dataviewCmd = &cobra.Command{
	Use:   "dataview",
	Short: "Manage template-generated regions in vault notes",
	Long: `Work with the template-generated regions in vault notes. This is a
container for subcommands: render refreshes a note's generated content,
lint checks the markers. The parent command itself does nothing — reach
for a subcommand.

Generated regions are sections of notes wrapped in VAULTMIND:GENERATED markers:

  <!-- VAULTMIND:GENERATED:{key}:START -->
  ... managed content ...
  <!-- VAULTMIND:GENERATED:{key}:END -->

Content between the markers is owned by section templates stored under
.vaultmind/sections/{type}/{key}.md. Running render replaces the content;
running lint catches marker errors before they cause a render failure.

SUBCOMMANDS

  lint    Scan every note for malformed or duplicated VAULTMIND:GENERATED
          markers. Run this after editing notes by hand or after a merge to
          confirm the marker structure is intact before rendering.

  render  Replace a note's generated region with fresh template output.
          Supports dry-run preview, unified diff output, and optional git
          commit of the result.

WHEN TO USE

  After editing templates                run: vaultmind dataview render <note>
  Before a bulk render pass              run: vaultmind dataview lint
  Confirming markers survived a merge    run: vaultmind dataview lint
  Previewing what render would change    run: vaultmind dataview render <note> --dry-run --diff

EXAMPLES

  vaultmind dataview lint --vault ./my-vault           # check all notes for marker errors
  vaultmind dataview render my-note-id                 # refresh generated region in one note
  vaultmind dataview render my-note-id --dry-run       # preview without writing
  vaultmind dataview render my-note-id --diff          # show unified diff of proposed changes
  vaultmind dataview render my-note-id --commit        # render and stage a git commit`,
}

func init() {
	RootCmd.AddCommand(dataviewCmd)
}
