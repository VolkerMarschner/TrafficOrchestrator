create a new minor version with the following changes:
When in agent mode, the agent recieves traffic generation tasks
from the master. once the agent recieves them, those instructions shall be stored in a local file called instructions.conf together with a timestamp when they were recieved. from then on, the agent enforces the instructions to send or recieve traffic from this file - a permanent connection to the master is not neccessary. With this config file in place, it shall be possible for an agent to work in a standalone way without a master. 
After a configurable interval (set by the master in an option called TTL, stored in the agents instructions.conf) the agent will ask the master for new instructions automatically. The master can send new instructions at any time and the agent must update accordingly.

In case the Agent is started as a non-root user, a warning shall be displayed to te user, and sent to the master, that only ports > 1024 are posssible to be configured. 

In case something is unclear - please ask questions!

Additional task:
The traffic generation itself seems not to work - please check the code on reasons, why opening Ports on one Agent and connecting to it from another agent could fail. If neccessary, send a random textstring as a payload in all packets.

Once finished:
write the current Version into a file named Version.txt
compile binaries for linux and windows
push the updated repo to github