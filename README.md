## Charging Planner for evcc

[evcc](https://evcc.io) is a great tool for flexibly charging your electrical vehicles. 
One of the rare missing features so far is a charging plan for the whole week, in order to guarantee that the car is charged for sure at a configured time.
In evcc you can set a single target time for the end of your loading session, but it is erased as soon as the session is over so that you have to re-enter it every day if you have a regular loading schedule.

But luckily and thanks to the good extensibility of evcc you can implement such a scheduler easy on your own. 

It works like this:

* When evcc detects that a vehicle gets connected it can send an event to external services that you can configure in the configuration file. 
* A program listening to the event can calculate the next end time of a charging session. By default evcc loads with the excess energy of your PV but with a given endpoint it ensures that your car reaches the configured State-of-Charge (SoC) until that end date. If not enough sun energy is available the remainign energy is taken from the grid (and if you are using a dynamic pricemodel for your current, it even selects the cheapest time to do so). The information of the desired end times can come e.g. from another static configuration file.
* The result (end time and target SoC) is then sent to evcc which then sets this end date.

In short, as soon as you connect your car to your wallbox, an external program will set the charging plan in evcc like you would do via the UI.

This is a Prove-of-Concept (which actually is another wording for no long-term support is planned) that leverages a static configuration file to define charging plan and needs an MQTT broker for communication with evcc. 

> [!IMPORTANT]  
> This project works only with the latest version of evcc that you need 
> to compile on your own from https://github.com/evcc-io/evcc
> There is no release yet.

Here's an example:

``` yaml

# MQTT configuration to connect to the broker. You can use the same 
# configuration options as for evcc itself. 
# See https://docs.evcc.io/docs/reference/configuration/mqtt
mqtt:
  broker: "broker.mqtt"
  user: "user"
  password: "pass"
  # Topics for connect to the evcc instance. The following 
  # values are the defaults and don't need to be changed usually.
  topics:
    # Topic to listen on for connect/disconnect events
    events: "evcc/events"
    # Topic to send the target charging time. Use %s as placeholder for the vehicle id
    planSoc: "evcc/vehicles/%s/planSoc"
# List of vehicles that should be monitored for getting connected    
vehicles:
  - name: "ioniq6"
    # Default SoC when not given in an individual entry
    soc: 80
    # Weekly schedule for charging
    schedule:
        # Day can be any weekday or "weekend" (Sat & Sun) 
        # or "workday" (Mon - Fri)
      - day: "Monday"
        # Time when the charging must be finished and the vehicle must have
        # reached its target SoC
        time: "07:00"
        # Target SoC, overriding the default defined above
        soc: 70
      - day: "Wednesday"
        time: "07:00"
        # No soc specified; will use global soc (80)
      - day: "Friday"
        time: "07:00"
        soc: 90
      - day: "workday"
        time: "08:00"
        # Applies to all workdays not already specified
      - day: "weekend"
        time: "09:00"
        soc: 60

```

The communication works via MQTT. For this to work, you need to setup the MQTT integration evcc correspondingly. Here are the relevant parts needed to connect the dots:

``` yaml
# Connect to the same MQTT broker as evcc-charging-planner
mqtt:
  broker: "broker.mqtt"
  user: "user"
  password: "pass"
  
# push messages
messaging:
  events:
    connect:
      # ${vehicleName} is replaced by the vehicle id that connects, 
      # ${mode} is the charging mode (e.g. "PV")
      # the type needs is "connect"
      # Keep the msg as is as evcc-charging-planner excepts it to be like 
      # this
      msg: '{"vehicle": "${vehicleName}", "type": "connect"}'
  services:
      # New "custom" service for sending out push message. 
      # Any plugin as described in https://docs.evcc.io/docs/reference/plugins
      # can be used
    - type: custom
      send:
        # Triggers MQTT plugins
        source: mqtt
        # Send all push event to this topic. Keep this as default.
        topic: "evcc/events"
```

### Future

For the future I could imagine that the simple planning logic could be integrated into upstream evcc. We will see.

Other ideas is to extend the way how to configure the schedule:

* Connect to Google Calendar and check for entries that indicate a business trip. Set the end charging time to beginning of such meetings (or with some buffer, but you get the idea)
* Integrate an AI Agentic Workflow to let the user specify the plan with natural language
* Examine weather forecasts and tariff forecast to optimize costs.


### Feedback

Please use GitHub issues to leave any feedback.

