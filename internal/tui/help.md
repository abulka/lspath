LSPATH HELP
===========

PURPOSE
-------
lspath is a terminal tool designed to help you understand, analyze, and optimize your system PATH. It provides a clear visualization of how your PATH is constructed by your shell's startup sequence.

FEATURES
--------
• Visualization: See exactly where each PATH entry comes from.
• Configuration Flow: Trace the execution of shell startup files (e.g., .zshrc, .zprofile).
• Diagnostics: Identify duplicate entries and missing directories.
• File Preview: Inspect the code in your config files that modifies the PATH.
• Directory Listing: View the contents of any directory in your PATH.
• Search: Find specific binaries within your PATH.

HOW TO USE
----------
1. Browse: Use arrow keys to navigate the list of PATH entries.
2. Details: View directory stats and listings in the right panel.
3. Flow Mode: Press 'f' to see the shell startup sequence.
4. Diagnostics: Press 'd' to see a detailed report of issues.

FLOW MODE
---------
Flow Mode allows you to visualize the "evolution" of your PATH. As your shell starts up, it executes various configuration files. Each file might add or change directories in your PATH. 
• Press 'f' to enter Flow Mode.
• Use the arrow keys to step through each configuration file.
• The list on the left will highlight which PATH entries were added at that specific step.
• Press 'c' to toggle "Cumulative View" - this shows you exactly what your PATH looked like at that point in time.

WHICH MODE
----------
Which Mode helps you find exactly where a command is coming from and if it is being "shadowed" by another version in a different directory.
• Press 'w' to enter Which Mode.
• Type the name of a command (e.g., 'python' or 'ls').
• lspath will filter the PATH list to show every directory that contains a file matching that name.
• The highlighted entries show you which version of the command would run first based on PATH priority.

WHY LSPATH?
-----------
Your system PATH determines which programs run when you type a command. A messy PATH can cause terminal sluggishness and command shadowing. lspath makes cleanup easy.


KEYBOARD SHORTCUTS
------------------

COMMON NAVIGATION
• ↑ / k       : Scroll up by a line
• ↓ / j       : Scroll down by a line
• PgUp / b    : Scroll up by page
• PgDn / Space: Scroll down by page
• Home / g    : Jump to top
• End / G     : Jump to bottom
• Tab         : Switch focus between panels

GLOBAL ACTIONS
• ? / h       : Toggle this help dialog
• f           : Toggle Flow Mode (visualize shell startup)
• d           : Toggle Diagnostics (show report)
• q / Ctrl+C  : Quit application
• Esc         : Close popups / Return to normal mode

MODE SPECIFIC
• w           : Run 'which' on a command (Which Mode)
• c           : Toggle Cumulative view (Flow Mode)


ABOUT
-----
lspath version 1.1.0
(c) Andy Bulka 2026
Feedback: abulka@gmail.com
