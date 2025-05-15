"""Dictionary organizing functions for creating sweep dictionaries for Qfuncs."""

from __future__ import annotations

from collections.abc import ItemsView, Mapping, Sequence
from typing import Any, overload

from ..datatypes import Channel, Gate, Ohmic


class Wdep:
    """Holds the dependancy information for a single pad."""

    channel: Channel | None
    dependancy: bool

    def __init__(
        self,
        channel: Channel | None,
        dependancy: bool,
    ):
        """Constructs a single dependancy object."""
        self.channel = channel
        self.dependancy = dependancy

    def set_dependancy(self) -> None:
        """Sets the dependancy flag."""
        self.dependancy = True


class Wdict:
    """Holds the waveform variables for a single pad."""

    amplitude: float
    v_start: float
    favorite: bool
    unit: str
    name: Gate | Ohmic | str

    # optional inputs
    dependancies: list[Wdep] | Wdep | None

    def __init__(
        self,
        amplitude: float,
        v_start: float,
        name: Gate | Ohmic,
        favorite: bool = False,
        unit: str = "V",
        dependancies: list[Wdep] | Wdep | None = None,
    ):
        """Constructs a single set of waveform variables."""
        self.amplitude = amplitude
        self.v_start = v_start
        self.favorite = favorite
        self.unit = unit
        self.name = name
        if dependancies is not None:
            self.dependancies = dependancies
        else:
            self.dependancies = None


class Waveform1D:
    """A Waveform1D object is the standard input to a AWG like device.

    Contains a starting voltage and amplitude for every channel. There are additional optional inputs besides those.

    Importantly contains a favorite indicator for flagging which channel is the most important to scale others from.
    """

    data: dict[Gate | Ohmic, Wdict]

    def __init__(self):
        """Constructs an empty Waveform1D object."""
        self.data = {}

    def __getitem__(self, key: Gate | Ohmic) -> Wdict:
        """Grabs the item stored inside of the data storage attribute."""
        return self.data[key]

    def __setitem__(self, key: Gate | Ohmic, value: Wdict) -> None:
        """Sets the item stored inside of the data storage attribute."""
        self.data[key] = value

    def keys(self) -> list[Gate | Ohmic]:
        """Grabs the gates and ohmics stored inside of the data storage attribute."""
        return list(self.data.keys())

    def items(self) -> ItemsView[Gate | Ohmic, Wdict]:
        """Grabs the items stored inside of the data storage attribute."""
        return self.data.items()

    def merge(
        self,
        other: Waveform1D,
        path=[],
    ) -> Waveform1D:
        """Merges other Waveform1D into this one.

        This is useful when combining multiple channelInputs for simultaneous sweeps together
        This overwrites the dictionary in a
        """
        for key in other.keys():
            if key in self.keys():
                if self[key] != other[key]:
                    raise Exception("Conflict at " + ".".join(path + [str(key)]))
            else:
                self[key] = other[key]
        return self

    def get_favorite(self) -> Gate | Ohmic:
        """Gets the favorite gate stored in the waveform.

        Returns:
            the favorited connection on the sample for the waveform

        Raises:
            ValueError if there is not 1 favorite connection
        """
        favorites = self.find_favorites()
        if len(favorites) != 1:
            raise ValueError(
                f"Incorrect number of favorites found. Found {len(favorites)}. There should be exactly 1."
            )
        return favorites[0]

    def get_dependancy(self, channel: Channel) -> Gate | Ohmic | None:
        """Gets the dependancy for the given channel. And if not present returns None."""
        dependancy = self.find_dependancy(channel=channel)
        if len(dependancy) > 1:
            raise ValueError(
                f"Incorrect number of dependancies found. Found {len(dependancy)}. There should be at most 1."
            )
        if len(dependancy):
            return dependancy[0]
        return None

    def get_dep_andor_favorite(self, channel: Channel) -> Gate | Ohmic:
        """Gets the dependancy if specified, or the favorite if not."""
        dependancy = self.get_dependancy(channel=channel)
        if dependancy is not None:
            return dependancy
        return self.get_favorite()

    def used_gates(self) -> list[Gate]:
        """Collects the list of gates stored in the object.

        Returns:
            List of Gate objects.
        """
        # Initialize an empty list to store the gates.
        values = []

        # Iterate over the items in the data storage attribute.
        for key, value in self.data.items():
            # Check if the key is an instance of Gate.
            if isinstance(key, Gate):
                # If it is, add it to the list of gates.
                values.append(key)

        # Return the list of gates.
        return values

    def used_pads(self) -> list[Gate | Ohmic]:
        """Compiles all of the pads used in ChannelInputs into a list.

        Returns:
            list containing all of the pads
        """
        gates = []
        for gate in list(self.keys()):
            gates.append(gate)
        return gates

    def find_favorites(self) -> list[Gate | Ohmic]:
        """Collects the list of favorites stored in the object."""
        values = []
        for key, value in self.data.items():
            if value.favorite:
                values.append(key)
        return values

    def favorite_greatest_amp(self) -> None:
        """Favorites the gate inside of waveform that is programmed to have the greatest amplitude.

        The favorite gate is the one that is saved for all measurements when stored inside of the database
        """
        # now favorite a gate that is moving
        big_amps: dict[Gate | Ohmic, float] = {}
        for gate in self.keys():
            value = self[gate].amplitude
            big_amps[gate] = abs(value)
        if len(big_amps) > 0:
            maximal_changing_gate = max(big_amps, key=lambda x: big_amps[x])
            self[maximal_changing_gate].favorite = True
            return
        raise ValueError("Nothing found to favorite")

    def indicate_dependancy(self, pad: Gate | Ohmic, channel: Channel) -> None:
        """Indicates that a parameter is dependent on another parameter.

        Args:
            pad : the parameter that is dependent on another
            channel : the channel that should demonstrate the dependancy

        Raises:
            ValueError if the parameter does not exist
            ValueError if there is no favorite
            ValueError if the pad does not have a dependancy
            ValueError if multiple dependancies exist with the selected channel for the pad
        """
        if pad not in self.keys():
            raise ValueError(f"{pad} does not exist in the waveform")
        if not len(self.find_favorites()):
            raise ValueError("No favorites are selected. Come back another time.")
        if (not isinstance(self[pad].dependancies, Wdep)) and not (
            isinstance(self[pad].dependancies, list)
        ):
            raise ValueError("Must set a Wdep first before setting dependancy.")
        if isinstance(self[pad].dependancies, list):
            values = self[pad].dependancies
            assert values is not None
            assert isinstance(values, list)
            foundChannel = False
            for dep in values:
                if not isinstance(dep, Wdep):
                    raise ValueError("Must set a Wdep first before setting dependancy.")
                if dep.channel == channel:
                    if foundChannel:
                        raise ValueError(
                            "Multiple dependancies found with the same channel."
                        )
                    foundChannel = True
        dependancies = self[pad].dependancies
        assert dependancies is not None
        if not isinstance(dependancies, list):
            dependancies = [dependancies]
        # now they are all in a list
        for dep in dependancies:
            if dep.channel == channel:
                dep.set_dependancy()

    def find_dependancies(self) -> list[Gate | Ohmic]:
        """Collects the list of dependant pads stored in the object."""
        values = []
        for key, value in self.data.items():
            if value.dependancies is not None:
                wdeps = value.dependancies
                if isinstance(wdeps, Wdep):
                    wdeps = [wdeps]
                for dep in wdeps:
                    if dep.dependancy:
                        values.append(key)
        return values

    def find_dependancy(self, channel: Channel) -> list[Gate | Ohmic]:
        """Collects the list of dependant pads stored in the object."""
        values = []
        for key, value in self.data.items():
            if value.dependancies is not None:
                wdeps = value.dependancies
                if isinstance(wdeps, Wdep):
                    wdeps = [wdeps]
                for dep in wdeps:
                    if dep.dependancy and dep.channel == channel:
                        values.append(key)
        return values


class WaveformMaker:
    """Allows class to use these methods pertaining to programming waveforms."""

    def __init__(self):
        """Constructs an empty WaveformMaker object."""
        pass

    def empty_dict_builder_for_waveform1D(
        self,
        pads: Sequence[Gate | Ohmic] | Gate | Ohmic,
        v_start: float,
        amplitude: float,
        channel_gate_relation: dict[Channel, list[Gate | Ohmic]] = {},
    ) -> Waveform1D:
        """Sets up a new Waveform1D object. Basic initiallization is every pas the same.

        Args:
            pads : the names of the gates that are being set can be
            v_start : default value of starting voltage for all channels (V)
            amplitude : default value of amplitude for all channels (V)
            channel_gate_relation : a dictionary of channels to gates.
        """
        wavedict = Waveform1D()
        dependancies = self._setup_empty_dependancies(
            pads=pads,
            channel_gate_relation=channel_gate_relation,
        )

        if isinstance(pads, Sequence):
            for pad in pads:
                dependant = None
                if pad in dependancies.keys():
                    dependant = dependancies[pad]
                wavedict[pad] = Wdict(
                    amplitude=amplitude,
                    v_start=v_start,
                    favorite=False,
                    unit="V",
                    name=pad,
                    dependancies=dependant,
                )
            return wavedict
        else:
            dependant = None
            if pads in dependancies.keys():
                dependant = dependancies[pads]
            wavedict[pads] = Wdict(
                amplitude=amplitude,
                v_start=v_start,
                favorite=False,
                unit="V",
                name=pads,
                dependancies=dependant,
            )
            return wavedict

    def _channel_gate_relation_to_gate_channel_relation(
        self,
        channel_gate_relation: Mapping[Channel, Sequence[Gate | Ohmic]],
    ) -> dict[Gate | Ohmic, list[Channel]]:
        """Converts channel_gate_relation to gate_channel_relation."""
        gate_channel_relation = {}
        for channel, gates in channel_gate_relation.items():
            for gate in gates:
                if gate in gate_channel_relation.keys():
                    gate_channel_relation[gate] += [channel]
                else:
                    gate_channel_relation[gate] = [channel]
        return gate_channel_relation

    def _setup_empty_dependancies(
        self,
        channel_gate_relation: Mapping[Channel, Sequence[Gate | Ohmic]],
        pads: Sequence[Gate | Ohmic] | Gate | Ohmic,
    ) -> dict[Gate | Ohmic, list[Wdep]]:
        """Sets up dependancies."""
        dependancies: dict[Gate | Ohmic, list[Wdep]] = {}
        if len(channel_gate_relation):
            gate_channel_relation = (
                self._channel_gate_relation_to_gate_channel_relation(
                    channel_gate_relation=channel_gate_relation
                )
            )
            # this means we can now build dependancies
            for gate, channel in gate_channel_relation.items():
                channels = channel
                if not isinstance(channels, list):
                    channels = [channels]
                for chan in channels:
                    if (
                        (isinstance(pads, Sequence) and gate in pads)
                        or isinstance(pads, Gate)
                        or isinstance(pads, Ohmic)
                    ):
                        if gate in dependancies.keys():
                            dependancies[gate] += [Wdep(channel=chan, dependancy=False)]
                        else:
                            dependancies[gate] = [Wdep(channel=chan, dependancy=False)]

        return dependancies

    @overload
    def big_dict_builder_for_waveform1D(
        self,
        v_start: Mapping[Gate | Ohmic, float],
        amplitudes: Mapping[Gate | Ohmic, float],
        channel_gate_relation: Mapping[Channel, Sequence[Gate | Ohmic]] = {},
    ) -> Waveform1D: ...

    @overload
    def big_dict_builder_for_waveform1D(
        self,
        v_start: Mapping[Gate, float],
        amplitudes: Mapping[Gate, float],
        channel_gate_relation: Mapping[Channel, Sequence[Gate]] = {},
    ) -> Waveform1D: ...

    @overload
    def big_dict_builder_for_waveform1D(
        self,
        v_start: Mapping[Ohmic, float],
        amplitudes: Mapping[Ohmic, float],
        channel_gate_relation: Mapping[Channel, Sequence[Ohmic]] = {},
    ) -> Waveform1D: ...

    def big_dict_builder_for_waveform1D(
        self,
        v_start: Mapping[Gate | Ohmic, float]
        | Mapping[Gate, float]
        | Mapping[Ohmic, float],
        amplitudes: Mapping[Gate | Ohmic, float]
        | Mapping[Gate, float]
        | Mapping[Ohmic, float],
        channel_gate_relation: Mapping[Channel, Sequence[Gate | Ohmic]] = {},
    ) -> Waveform1D:
        """Builds entire Waveform1D dict.

        Args:
            v_start (dict[Gate, float]): indexed by gate, contains v_start sweep values [V]
            amplitudes (dict[Gate, float]): indexed by gate, contains amplitude sweep values [V]
            channel_gate_relation (dict[Channel, list[Gate | Ohmic]], optional): indexed by channel, contains the gates that are dependent on the channel. Defaults to {}.

        Returns:
            Waveform1D: dictionary indexed by gate containing all the information for Waveform1D
        """
        wavedict = Waveform1D()
        dependancies = self._setup_empty_dependancies(
            pads=list(v_start.keys()),
            channel_gate_relation=channel_gate_relation,
        )
        for gate, start_voltage in v_start.items():
            dependant = None
            if gate in dependancies.keys():
                dependant = dependancies[gate]
            wavedict[gate] = Wdict(
                amplitude=amplitudes[gate],  # type: ignore
                v_start=start_voltage,
                favorite=False,
                unit="V",
                name=gate,
                dependancies=dependant,
            )
        return wavedict

    def empty_sweep_builder_for_waveform1D(
        self,
        waveform: int,
        repetitions: int,
        nsteps: int = 10,
        step_length: int | None = 50,
        period: int = 50,
        duty_cycle: int = 50,
        slope: float = 11,
    ) -> dict[str, Any]:
        """Builds empty Waveform1D sweep dictionary.

        Args:
            waveform : integer for the waveform to be programmed

            The supported wave forms are:
                sine = 1
                square = 2
                traingle = 3
                staircase = 4
                awg = 5

            repetitions : number of repitions of waveform
            nsteps : number of steps, used only for staircase
            step_length : width of each step of the staircase, used only for staircase
            period: the period of the waveform, used only for triange, sine and square waves
            duty_cycle: percentage on or off, used only for square
            slope : slope of the ramping (V/s)

        Returns:
            dictionary containing all the information for Waveform1D
        """
        return {
            "waveform": waveform,
            "repetitions": repetitions,
            "nsteps": nsteps,
            "step_length": step_length,
            "period": period,
            "duty_cycle": duty_cycle,
            "slope": slope,
        }

    def sweep_compiler_for_sweep2DTurbo(
        self, fastnsteps, step_length_fast, slownsteps, fastslope, slowslope
    ):
        """Compile the sweep parameters for a 2D turbo sweep.

        Args:
            fastnsteps (int): Number of steps for the fast waveform sweep (AWG).
            step_length_fast (float): Step length in milliseconds for the fast sweep.
            slownsteps (int): Number of steps for the slow waveform sweep (staircase).
            fastslope (float): The ramping slope selected when programming the fast sweep in the AWG. This is in units of 11V/s.
            slowslope (float): The ramping slope selected when programming the slow sweep in the AWG. This is in units of 11V/s.

        Returns:
            tuple: A tuple containing the parameters for the fast and slow sweeps.
        """
        Channel2SweepParams = self.empty_sweep_builder_for_waveform1D(
            waveform=4,
            repetitions=1,
            nsteps=slownsteps,
            step_length=None,
            slope=slowslope,
        )
        Channel1SweepParams = self.empty_sweep_builder_for_waveform1D(
            waveform=5,
            repetitions=slownsteps,
            nsteps=fastnsteps,
            step_length=step_length_fast,
            slope=fastslope,
        )
        return Channel1SweepParams, Channel2SweepParams
